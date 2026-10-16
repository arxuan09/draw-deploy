#!/usr/bin/env python3
"""draw_admin.py — 站点运维命令行（draw-site-admin 技能自带）。

用后台「系统 → 运维密钥」签发的密钥调用站点的 /api/admin/* 接口。
只用 Python 3.8+ 标准库，单文件，源码即文档：想知道某条命令发了什么请求，直接读对应的 cmd_* 函数。

    python3 draw_admin.py login --base-url https://your-site.com     # 隐藏输入密钥
    python3 draw_admin.py doctor                                     # 站点体检
    python3 draw_admin.py settings set site_name=我的站
    python3 draw_admin.py call models/list --json                    # 任何后台接口

所有命令都支持 --json：输出 {"ok": true, "data": …} 或 {"ok": false, "error": {"code", "params", "message"}}。
退出码：0 成功 / 1 失败 / 2 参数错误 / 3 认证或权限 / 6 上游或网络异常（可重试）/ 7 超时 / 8 需要确认（加 --yes）。
"""

import argparse
import datetime as _dt
import difflib
import getpass
import json
import mimetypes
import os
import socket
import ssl
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid

VERSION = "1.1.0"

EXIT_OK = 0
EXIT_FAIL = 1
EXIT_USAGE = 2
EXIT_AUTH = 3
EXIT_UPSTREAM = 6
EXIT_TIMEOUT = 7
EXIT_CONFIRM = 8

TOKEN_PREFIX = "draw_ak_"
SECRET_MASK = "********"


class CLIError(Exception):
    def __init__(self, message, code="cli.error", exit_code=EXIT_FAIL, params=None):
        super().__init__(message)
        self.message = message
        self.code = code
        self.exit_code = exit_code
        self.params = params or {}


# ---------------------------------------------------------------------------
# Config: ~/.config/draw-admin/config.json (Windows: %APPDATA%\draw-admin)
# ---------------------------------------------------------------------------


def config_path():
    if os.name == "nt" and os.environ.get("APPDATA"):
        base = os.path.join(os.environ["APPDATA"], "draw-admin")
    else:
        base = os.path.join(os.environ.get("XDG_CONFIG_HOME") or os.path.expanduser("~/.config"), "draw-admin")
    return os.path.join(base, "config.json")


def load_config():
    path = config_path()
    try:
        with open(path, "r", encoding="utf-8") as f:
            cfg = json.load(f)
    except FileNotFoundError:
        cfg = {}
    except (OSError, ValueError) as e:
        raise CLIError("配置文件无法读取：%s（%s）" % (path, e))
    cfg.setdefault("profiles", {})
    return cfg


def save_config(cfg):
    path = config_path()
    os.makedirs(os.path.dirname(path), exist_ok=True)
    tmp = path + ".tmp"
    fd = os.open(tmp, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(fd, "w", encoding="utf-8") as f:
        json.dump(cfg, f, ensure_ascii=False, indent=2)
    os.replace(tmp, path)
    try:
        os.chmod(path, 0o600)
    except OSError:
        pass


LOCAL_HOSTS = ("localhost", "127.0.0.1", "::1")


def normalize_base_url(url):
    url = (url or "").strip().rstrip("/")
    if not url.startswith(("https://", "http://")):
        raise CLIError("站点地址要以 https:// 开头：%s" % url, exit_code=EXIT_USAGE)
    if url.startswith("http://"):
        host = urllib.parse.urlsplit(url).hostname or ""
        if host not in LOCAL_HOSTS and os.environ.get("DRAW_ALLOW_HTTP") != "1":
            raise CLIError(
                "密钥不能明文发送：站点地址请用 https://。只在内网测试时可设环境变量 DRAW_ALLOW_HTTP=1。",
                code="cli.insecureUrl",
                exit_code=EXIT_USAGE,
            )
    return url


def resolve_target(args):
    """Returns (profile_name, base_url, key). Env vars win over the config file."""
    env_url = os.environ.get("DRAW_BASE_URL")
    env_key = os.environ.get("DRAW_ADMIN_KEY")
    cfg = load_config()
    name = args.profile or os.environ.get("DRAW_ADMIN_PROFILE") or cfg.get("default")
    prof = cfg["profiles"].get(name, {}) if name else {}
    if args.profile and not prof and not (env_url and env_key):
        raise CLIError("没有名为「%s」的配置档，先运行 login --profile %s" % (args.profile, args.profile), exit_code=EXIT_USAGE)
    base_url = env_url or prof.get("baseUrl")
    key = env_key or prof.get("adminKey")
    if not base_url or not key:
        raise CLIError(
            "还没登录。请运营方在自己的终端运行：python3 %s login --base-url https://站点地址" % script_path(),
            code="cli.notLoggedIn",
            exit_code=EXIT_AUTH,
        )
    return name, normalize_base_url(base_url), key


def script_path():
    return os.path.abspath(sys.argv[0]) if sys.argv and sys.argv[0] else "draw_admin.py"


# ---------------------------------------------------------------------------
# HTTP
# ---------------------------------------------------------------------------


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    """urllib would follow a redirect with the Authorization header still on,
    handing the key to whatever host the response points at. Refuse instead."""

    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


_OPENER = urllib.request.build_opener(_NoRedirect(), urllib.request.HTTPSHandler(context=ssl.create_default_context()))


class Client:
    def __init__(self, base_url, key, timeout=60):
        self.base_url = base_url
        self.key = key
        self.timeout = timeout
        self._catalog = None

    def _headers(self):
        return {
            "Authorization": "Bearer " + self.key,
            "X-Draw-Admin-CLI": VERSION,
            "User-Agent": "draw-admin-cli/" + VERSION,
            "Accept": "application/json",
        }

    def post(self, path, body=None):
        data = json.dumps(body if body is not None else {}, ensure_ascii=False).encode("utf-8")
        headers = self._headers()
        headers["Content-Type"] = "application/json"
        return self._send(path, data, headers)

    def upload(self, path, file_path, fields=None):
        try:
            with open(file_path, "rb") as f:
                content = f.read()
        except OSError as e:
            raise CLIError("读不了文件：%s（%s）" % (file_path, e), exit_code=EXIT_USAGE)
        boundary = "----drawadmin" + uuid.uuid4().hex
        ctype = mimetypes.guess_type(file_path)[0] or "application/octet-stream"
        parts = []
        for k, v in (fields or {}).items():
            parts.append(
                ("--%s\r\nContent-Disposition: form-data; name=\"%s\"\r\n\r\n%s\r\n" % (boundary, k, v)).encode("utf-8")
            )
        filename = os.path.basename(file_path).replace('"', "")
        parts.append(
            (
                "--%s\r\nContent-Disposition: form-data; name=\"file\"; filename=\"%s\"\r\nContent-Type: %s\r\n\r\n"
                % (boundary, filename, ctype)
            ).encode("utf-8")
        )
        parts.append(content)
        parts.append(("\r\n--%s--\r\n" % boundary).encode("utf-8"))
        headers = self._headers()
        headers["Content-Type"] = "multipart/form-data; boundary=" + boundary
        return self._send(path, b"".join(parts), headers)

    def _send(self, path, data, headers):
        path = path.strip().lstrip("/")
        if path.startswith("api/admin/"):
            path = path[len("api/admin/") :]
        url = "%s/api/admin/%s" % (self.base_url, path)
        req = urllib.request.Request(url, data=data, headers=headers, method="POST")
        try:
            with _OPENER.open(req, timeout=self.timeout) as resp:
                status = resp.status
                raw = resp.read()
        except urllib.error.HTTPError as e:
            if 300 <= e.code < 400:
                raise CLIError(
                    "站点返回了重定向（HTTP %d → %s）。为了不把密钥发到别的地址，CLI 不跟随重定向；请把站点地址改成最终地址后重新 login。"
                    % (e.code, e.headers.get("Location")),
                    code="cli.redirect",
                    exit_code=EXIT_FAIL,
                )
            status = e.code
            raw = e.read()
        except (socket.timeout, TimeoutError):
            raise CLIError("请求超时：%s" % url, code="cli.timeout", exit_code=EXIT_TIMEOUT)
        except urllib.error.URLError as e:
            reason = e.reason
            if isinstance(reason, (socket.timeout, TimeoutError)):
                raise CLIError("请求超时：%s" % url, code="cli.timeout", exit_code=EXIT_TIMEOUT)
            raise CLIError("连不上站点 %s：%s" % (self.base_url, reason), code="cli.network", exit_code=EXIT_UPSTREAM)
        try:
            payload = json.loads(raw.decode("utf-8") or "{}")
        except ValueError:
            snippet = raw[:200].decode("utf-8", "replace")
            raise CLIError(
                "站点返回的不是 JSON（HTTP %d）：%s" % (status, snippet),
                code="cli.badResponse",
                exit_code=EXIT_UPSTREAM if status >= 500 else EXIT_FAIL,
            )
        if status >= 400 or payload.get("ok") is False:
            code = payload.get("errorCode") or "http.%d" % status
            message = payload.get("error") or "HTTP %d" % status
            params = payload.get("errorParams") or {}
            if status in (401, 403):
                exit_code = EXIT_AUTH
            elif status == 429 or status >= 500:
                exit_code = EXIT_UPSTREAM
            elif status == 400 and code == "errors.validation":
                exit_code = EXIT_USAGE
            else:
                exit_code = EXIT_FAIL
            raise CLIError(message, code=code, exit_code=exit_code, params=params)
        payload.pop("ok", None)
        return payload

    def catalog(self):
        if self._catalog is None:
            res = self.post("cli/catalog")
            self._catalog = {r["path"]: r for r in res.get("routes", [])}
        return self._catalog


# ---------------------------------------------------------------------------
# Output
# ---------------------------------------------------------------------------


def dumps(obj):
    return json.dumps(obj, ensure_ascii=False, indent=2)


def emit(args, data, human=None):
    if args.json:
        print(dumps({"ok": True, "data": data}))
    elif human is not None:
        human(data)
    else:
        print(dumps(data))


def warn(msg):
    print("提示：" + msg, file=sys.stderr)


def fmt_value(v):
    if v is None:
        return ""
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (dict, list)):
        return json.dumps(v, ensure_ascii=False)
    s = str(v)
    if "\n" in s or len(s) > 80:
        first = s.splitlines()[0] if s else ""
        return "%s…（%d 字）" % (first[:60], len(s))
    return s


def table(rows, columns):
    """rows: list of dicts; columns: list of (key, title)."""
    if not rows:
        print("（空）")
        return
    cells = [[fmt_value(r.get(k)) for k, _ in columns] for r in rows]
    widths = [max(len(t), *(len(c[i]) for c in cells)) for i, (_, t) in enumerate(columns)]
    print("  ".join(t.ljust(widths[i]) for i, (_, t) in enumerate(columns)).rstrip())
    for c in cells:
        print("  ".join(c[i].ljust(widths[i]) for i in range(len(columns))).rstrip())


def confirm(args, summary):
    """Asks before a route the catalog marks confirm. --yes skips; with no
    terminal the command stops with exit code 8 so the Agent asks the human."""
    if args.yes:
        return
    if sys.stdin.isatty() and not args.json:
        answer = input("将要：%s\n确认执行？[y/N] " % summary).strip().lower()
        if answer in ("y", "yes"):
            return
        raise CLIError("已取消", code="cli.cancelled", exit_code=EXIT_CONFIRM)
    raise CLIError(
        "这个操作需要确认：%s。征得运营方同意后加 --yes 重新运行。" % summary,
        code="cli.confirmRequired",
        exit_code=EXIT_CONFIRM,
    )


def confirm_route(args, client, path, summary=None):
    route = client.catalog().get(path)
    if route is None:
        raise CLIError("站点没有这个后台接口：%s（用 routes 命令查看全部）" % path, code="cli.unknownRoute", exit_code=EXIT_USAGE)
    if route.get("confirm"):
        confirm(args, summary or route.get("summary") or path)
    return route


# ---------------------------------------------------------------------------
# Commands: auth & profiles
# ---------------------------------------------------------------------------


def client_for(args):
    _, base_url, key = resolve_target(args)
    return Client(base_url, key, timeout=args.timeout)


def check_version(data):
    need = ((data or {}).get("cli") or {}).get("minVersion")
    if need and version_tuple(need) > version_tuple(VERSION):
        warn("脚本版本 %s 低于站点要求的 %s，请从部署仓重新复制 draw-site-admin 技能。" % (VERSION, need))


def version_tuple(v):
    out = []
    for part in str(v).split("."):
        try:
            out.append(int(part))
        except ValueError:
            out.append(0)
    return tuple(out)


def cmd_login(args):
    base_url = normalize_base_url(args.base_url or os.environ.get("DRAW_BASE_URL") or "")
    if args.key_stdin:
        key = sys.stdin.readline().strip()
    elif sys.stdin.isatty():
        key = getpass.getpass("粘贴运维密钥（输入不显示）：").strip()
    else:
        raise CLIError(
            "login 要在终端里由运营方本人运行（密钥用隐藏输入，不经过对话）。脚本化场景用 --key-stdin 或环境变量 DRAW_ADMIN_KEY。",
            code="cli.noTerminal",
            exit_code=EXIT_USAGE,
        )
    if not key.startswith(TOKEN_PREFIX):
        raise CLIError("这不是运维密钥（应以 %s 开头）" % TOKEN_PREFIX, exit_code=EXIT_USAGE)
    client = Client(base_url, key, timeout=args.timeout)
    who = client.post("cli/whoami")
    cfg = load_config()
    name = args.profile or base_url.split("://", 1)[1].split("/")[0]
    cfg["profiles"][name] = {"baseUrl": base_url, "adminKey": key}
    if args.profile is None or not cfg.get("default"):
        cfg["default"] = name
    save_config(cfg)
    check_version(who)

    def human(d):
        k = d.get("key") or {}
        print("已登录 %s（配置档 %s）" % (base_url, name))
        print("签发人：%s · 密钥：%s · 权限：%s" % (d["user"]["name"], k.get("name"), "、".join(k.get("scopes") or [])))
        print("配置保存在 %s" % config_path())

    emit(args, {"profile": name, **who}, human)


def cmd_logout(args):
    cfg = load_config()
    name = args.profile or cfg.get("default")
    if not name or name not in cfg["profiles"]:
        raise CLIError("没有可退出的配置档", exit_code=EXIT_USAGE)
    del cfg["profiles"][name]
    if cfg.get("default") == name:
        cfg["default"] = next(iter(cfg["profiles"]), None)
    save_config(cfg)
    emit(args, {"removed": name}, lambda d: print("已删除配置档 %s（密钥本身仍有效，不用了请到后台吊销）" % name))


def cmd_profiles(args):
    cfg = load_config()
    rows = [
        {"name": n, "baseUrl": p.get("baseUrl"), "key": (p.get("adminKey") or "")[: len(TOKEN_PREFIX) + 8] + "_…", "default": n == cfg.get("default")}
        for n, p in cfg["profiles"].items()
    ]
    emit(args, rows, lambda d: table(d, [("name", "配置档"), ("baseUrl", "站点"), ("key", "密钥"), ("default", "默认")]))


def cmd_use(args):
    cfg = load_config()
    if args.name not in cfg["profiles"]:
        raise CLIError("没有名为「%s」的配置档" % args.name, exit_code=EXIT_USAGE)
    cfg["default"] = args.name
    save_config(cfg)
    emit(args, {"default": args.name}, lambda d: print("默认配置档改为 %s" % args.name))


def cmd_whoami(args):
    client = client_for(args)
    data = client.post("cli/whoami")
    check_version(data)

    def human(d):
        k = d.get("key") or {}
        site = d.get("site") or {}
        print("站点：%s  %s（币种 %s）" % (site.get("name"), site.get("publicUrl") or client.base_url, site.get("currency")))
        print("签发人：%s <%s>" % (d["user"]["name"], d["user"].get("email") or "-"))
        print("密钥：%s  draw_ak_%s_…" % (k.get("name"), k.get("prefix")))
        print("权限：%s" % "、".join(k.get("scopes") or []))
        print("过期：%s" % (k.get("expiresAt") or "永不过期"))
        print("服务端版本：%s · 脚本版本：%s" % ((d.get("server") or {}).get("version"), VERSION))

    emit(args, data, human)


# ---------------------------------------------------------------------------
# Commands: doctor, routes, call
# ---------------------------------------------------------------------------

STATUS_TAG = {"ok": "[OK]  ", "missing": "[缺失]", "warn": "[注意]"}
LEVEL_TITLE = {"required": "必须", "recommended": "推荐", "optional": "可选"}


def cmd_doctor(args):
    data = client_for(args).post("setup/checklist")

    def human(d):
        items = d.get("items") or []
        for level in ("required", "recommended", "optional"):
            group = [it for it in items if it.get("level") == level]
            if not group:
                continue
            print("== %s ==" % LEVEL_TITLE[level])
            for it in group:
                print("%s %s：%s" % (STATUS_TAG.get(it.get("status"), it.get("status")), it.get("title"), it.get("detail")))
                if it.get("status") != "ok":
                    fix = it.get("fix") or {}
                    if fix.get("settings"):
                        print("        设置：%s" % ", ".join(fix["settings"]))
                    if fix.get("verify"):
                        print("        验证：%s" % fix["verify"])
                    if fix.get("adminPath"):
                        print("        后台：%s" % fix["adminPath"])
            print()
        s = d.get("summary") or {}
        print(
            "必须项 %d/%d 完成，推荐项 %d/%d 完成"
            % (s.get("requiredDone", 0), s.get("required", 0), s.get("recommendedDone", 0), s.get("recommended", 0))
        )

    emit(args, data, human)


def cmd_routes(args):
    client = client_for(args)
    routes = sorted(client.catalog().values(), key=lambda r: r["path"])
    if args.scope:
        routes = [r for r in routes if r.get("scope") == args.scope]
    emit(
        args,
        routes,
        lambda d: table(d, [("path", "接口"), ("scope", "档位"), ("confirm", "需确认"), ("summary", "说明")]),
    )


def route_path(raw):
    path = raw.strip().lstrip("/")
    if path.startswith("api/admin/"):
        path = path[len("api/admin/") :]
    return path


def format_fields(fields, indent="  "):
    lines = []
    for f in fields or []:
        opt = "（可为空）" if f.get("optional") else ""
        lines.append("%s%s: %s%s" % (indent, f.get("name"), f.get("type"), opt))
        if f.get("fields"):
            lines.extend(format_fields(f["fields"], indent + "    "))
    return lines


def cmd_describe(args):
    client = client_for(args)
    path = route_path(args.path)
    route = client.catalog().get(path)
    if route is None:
        close = difflib.get_close_matches(path, list(client.catalog()), n=5)
        hint = "，是不是：%s" % "、".join(close) if close else ""
        raise CLIError("站点没有这个后台接口：%s%s" % (path, hint), code="cli.unknownRoute", exit_code=EXIT_USAGE)

    def human(r):
        print("%s  —  %s" % (r["path"], r.get("summary")))
        print("权限档：%s%s" % (r.get("scope"), "，需要确认（--yes）" if r.get("confirm") else ""))
        body = r.get("body")
        if body is None:
            print("请求体：未知（先 list / get 看数据结构再组装）")
        elif body.get("kind") == "none":
            print("请求体：无")
        elif body.get("kind") == "multipart":
            print("请求体：multipart 上传，用 call %s --file <文件>；其它表单字段放 --data：" % r["path"])
            print("\n".join(format_fields(body.get("fields"))))
        else:
            print("请求体（JSON）：")
            print("\n".join(format_fields(body.get("fields"))))

    emit(args, route, human)


def warn_unknown_fields(route, data):
    """Field names the handler does not read are silently dropped by the
    server; say so before sending."""
    body = route.get("body")
    if not body or not isinstance(data, dict):
        return
    if body.get("kind") == "none":
        if data:
            warn("这个接口不读请求体，--data 会被忽略")
        return
    known = {f.get("name") for f in body.get("fields") or []}
    unknown = [k for k in data if k not in known]
    if unknown:
        warn("接口不认识这些字段，会被忽略：%s。可用字段：%s（describe %s 查看）" % ("、".join(unknown), "、".join(sorted(known)), route.get("path")))


def read_data_arg(args):
    if args.data and args.data_file:
        raise CLIError("--data 和 --data-file 只能用一个", exit_code=EXIT_USAGE)
    raw = args.data
    if args.data_file:
        try:
            with open(args.data_file, "r", encoding="utf-8") as f:
                raw = f.read()
        except OSError as e:
            raise CLIError("读不了 %s：%s" % (args.data_file, e), exit_code=EXIT_USAGE)
    if not raw:
        return {}
    try:
        return json.loads(raw)
    except ValueError as e:
        raise CLIError("请求体不是合法 JSON：%s" % e, exit_code=EXIT_USAGE)


def cmd_call(args):
    client = client_for(args)
    path = route_path(args.path)
    route = confirm_route(args, client, path)
    if route.get("scope") == "never":
        raise CLIError("「%s」只能在网页后台操作" % route.get("summary"), code="errors.apiKey.routeDenied", exit_code=EXIT_AUTH)
    payload = read_data_arg(args)
    warn_unknown_fields(route, payload)
    if args.file:
        data = client.upload(path, args.file, payload or None)
    else:
        data = client.post(path, payload)
    emit(args, data)


# ---------------------------------------------------------------------------
# Commands: settings
# ---------------------------------------------------------------------------

TRUE_WORDS = {"true", "1", "yes", "on", "y", "是", "开"}
FALSE_WORDS = {"false", "0", "no", "off", "n", "否", "关", ""}


def settings_index(client):
    res = client.post("settings/list")
    return {s["key"]: s for s in res.get("settings", [])}, res


def coerce(setting, raw):
    typ = setting.get("type")
    if typ == "boolean":
        low = str(raw).strip().lower()
        if low in TRUE_WORDS:
            return True
        if low in FALSE_WORDS:
            return False
        raise CLIError("%s 是开关，只能填 true / false：%s" % (setting["key"], raw), exit_code=EXIT_USAGE)
    if typ == "number":
        try:
            f = float(raw)
        except ValueError:
            raise CLIError("%s 要填数字：%s" % (setting["key"], raw), exit_code=EXIT_USAGE)
        return int(f) if f.is_integer() else f
    return raw


def unknown_key(key, index):
    close = difflib.get_close_matches(key, list(index), n=3)
    hint = "，是不是：%s" % "、".join(close) if close else ""
    return CLIError("没有这个设置项：%s%s" % (key, hint), code="cli.unknownSetting", exit_code=EXIT_USAGE)


def show_setting_value(s):
    if s.get("isSecret"):
        return "已设置" if s.get("value") else "未设置"
    return fmt_value(s.get("value"))


def cmd_settings_list(args):
    index, _ = settings_index(client_for(args))
    rows = [s for s in index.values() if not args.group or s.get("group") == args.group]
    if args.json:
        emit(args, rows)
        return
    group = None
    for s in rows:
        if s.get("group") != group:
            group = s.get("group")
            print("\n== %s ==" % group)
        print("%s = %s    # %s%s" % (s["key"], show_setting_value(s), s.get("label"), "（密钥）" if s.get("isSecret") else ""))


def cmd_settings_get(args):
    index, _ = settings_index(client_for(args))
    out = []
    for key in args.keys:
        if key not in index:
            raise unknown_key(key, index)
        out.append(index[key])
    if args.json:
        emit(args, out)
        return
    for s in out:
        print("%s = %s" % (s["key"], show_setting_value(s)))
        print("    %s · %s · %s" % (s.get("label"), s.get("group"), s.get("type")))
        if s.get("description"):
            print("    " + s["description"])


def read_single_value(args, key):
    sources = [x for x in (args.secret, args.from_env, args.from_file) if x]
    if len(sources) != 1:
        raise CLIError("%s 没给值：用 %s=值，或 --secret / --from-env 变量名 / --from-file 文件" % (key, key), exit_code=EXIT_USAGE)
    if args.secret:
        if not sys.stdin.isatty():
            raise CLIError(
                "--secret 要在终端里由运营方本人运行（隐藏输入）。没有终端时请运营方先 export 一个环境变量，再用 --from-env。",
                code="cli.noTerminal",
                exit_code=EXIT_USAGE,
            )
        value = getpass.getpass("%s 的值（输入不显示）：" % key)
        again = getpass.getpass("再输入一次：")
        if value != again:
            raise CLIError("两次输入不一致", exit_code=EXIT_USAGE)
        return value
    if args.from_env:
        if args.from_env not in os.environ:
            raise CLIError("环境变量 %s 不存在" % args.from_env, exit_code=EXIT_USAGE)
        return os.environ[args.from_env]
    try:
        with open(args.from_file, "r", encoding="utf-8") as f:
            return f.read().rstrip("\n")
    except OSError as e:
        raise CLIError("读不了 %s：%s" % (args.from_file, e), exit_code=EXIT_USAGE)


def cmd_settings_set(args):
    client = client_for(args)
    index, _ = settings_index(client)
    values = {}
    for item in args.assignments:
        if "=" in item:
            key, raw = item.split("=", 1)
            key = key.strip()
            if key not in index:
                raise unknown_key(key, index)
            if index[key].get("isSecret"):
                raise CLIError(
                    "%s 是密钥类设置，不要写在命令行里：用 --secret（终端隐藏输入）、--from-env 或 --from-file" % key,
                    code="cli.secretOnCommandLine",
                    exit_code=EXIT_USAGE,
                )
            values[key] = coerce(index[key], raw)
        else:
            key = item.strip()
            if key not in index:
                raise unknown_key(key, index)
            if len(args.assignments) != 1:
                raise CLIError("--secret / --from-env / --from-file 一次只能设一个键", exit_code=EXIT_USAGE)
            values[key] = coerce(index[key], read_single_value(args, key))
    changes = []
    for key, new in values.items():
        s = index[key]
        old = s.get("value")
        if s.get("isSecret"):
            changes.append({"key": key, "label": s.get("label"), "from": "已设置" if old else "未设置", "to": "新值已设置" if new else "清空", "secret": True})
        elif old != new:
            changes.append({"key": key, "label": s.get("label"), "from": old, "to": new})
    unchanged = [k for k in values if k not in {c["key"] for c in changes}]
    result = {"changes": changes, "unchanged": unchanged, "saved": False, "dryRun": bool(args.dry_run)}
    if changes and not args.dry_run:
        client.post("settings/save", {"settings": {c["key"]: values[c["key"]] for c in changes}})
        result["saved"] = True

    def human(d):
        for c in d["changes"]:
            print("%s（%s）：%s → %s" % (c["key"], c["label"], fmt_value(c["from"]) or "（空）", fmt_value(c["to"]) or "（空）"))
        if d["unchanged"]:
            print("没有变化：%s" % ", ".join(d["unchanged"]))
        if d["dryRun"]:
            print("（--dry-run，未保存）")
        elif d["saved"]:
            print("已保存")

    emit(args, result, human)


def cmd_settings_test_mail(args):
    data = client_for(args).post("settings/test-mail", {"to": args.to})
    emit(args, data, lambda d: print("测试邮件已发往 %s，请到收件箱（含垃圾箱）确认" % args.to))


def cmd_settings_test_storage(args):
    data = client_for(args).post("settings/test-storage")
    st = data.get("storage") or {}
    if not st.get("ok"):
        raise CLIError("存储测试失败：%s" % st.get("message"), code="cli.storageTestFailed")
    emit(args, data, lambda d: print(st.get("message")))


def upload_and_set(args, route, key):
    client = client_for(args)
    up = client.upload(route, args.file)
    url = up.get("url")
    if not url:
        raise CLIError("上传成功但没拿到地址：%s" % dumps(up))
    client.post("settings/save", {"settings": {key: url}})
    emit(args, {"url": url, "setting": key}, lambda d: print("已上传并写入 %s = %s" % (key, url)))


def cmd_settings_upload_favicon(args):
    upload_and_set(args, "settings/upload-favicon", "site_favicon_url")


def cmd_settings_upload_share_image(args):
    upload_and_set(args, "settings/upload-share-image", "site_share_image_url")


# ---------------------------------------------------------------------------
# Commands: providers, models, health, packages
# ---------------------------------------------------------------------------


def on_off(value):
    low = value.lower()
    if low in ("on", "enable", "true", "1"):
        return True
    if low in ("off", "disable", "false", "0"):
        return False
    raise CLIError("只能填 on 或 off：%s" % value, exit_code=EXIT_USAGE)


def cmd_providers_list(args):
    data = client_for(args).post("providers/list")
    rows = data.get("providers") or []
    emit(args, rows, lambda d: table(d, [("key", "key"), ("name", "名称"), ("enabled", "启用"), ("baseUrl", "Base URL")]))


def read_secret_option(args, label):
    """The value of --secret / --from-env for a write-only field, or None."""
    if args.secret and args.from_env:
        raise CLIError("--secret 和 --from-env 只能用一个", exit_code=EXIT_USAGE)
    if args.from_env:
        if args.from_env not in os.environ:
            raise CLIError("环境变量 %s 不存在" % args.from_env, exit_code=EXIT_USAGE)
        return os.environ[args.from_env].strip()
    if args.secret:
        if not sys.stdin.isatty():
            raise CLIError(
                "--secret 要在终端里由运营方本人运行（隐藏输入）。没有终端时请运营方先 export 一个环境变量，再用 --from-env。",
                code="cli.noTerminal",
                exit_code=EXIT_USAGE,
            )
        return getpass.getpass("%s（输入不显示）：" % label).strip()
    return None


def find_provider(client, key):
    for p in client.post("providers/list").get("providers") or []:
        if p.get("key") == key:
            return p
    raise CLIError("没有 key 为 %s 的供应商（providers list 查看）" % key, exit_code=EXIT_USAGE)


def cmd_providers_create(args):
    client = client_for(args)
    body = {
        "name": args.name,
        "baseUrl": args.base_url,
        "description": args.description or "",
        "enabled": True,
    }
    if args.concurrency is not None:
        body["concurrencyLimit"] = args.concurrency
    api_key = read_secret_option(args, "%s 的 API Key" % args.name)
    if api_key:
        body["apiKey"] = api_key
    key = client.post("providers/create", body).get("providerKey")

    def human(d):
        print("已创建供应商 %s，标识 %s（%s）" % (args.name, key, args.base_url))
        if not api_key:
            print("还没设 API Key：请运营方在自己终端运行 providers set-key %s --secret" % key)

    emit(args, {"key": key, "name": args.name, "apiKeySet": bool(api_key)}, human)


def cmd_providers_set_key(args):
    client = client_for(args)
    api_key = read_secret_option(args, "%s 的 API Key" % args.key)
    if not api_key:
        raise CLIError("用 --secret（终端隐藏输入）或 --from-env 变量名 提供 API Key", exit_code=EXIT_USAGE)
    p = find_provider(client, args.key)
    body = {
        "providerKey": args.key,
        "name": p.get("name") or args.key,
        "baseUrl": p.get("baseUrl") or "",
        "description": p.get("description") or "",
        "enabled": bool(p.get("enabled")),
        "sortOrder": p.get("sortOrder") or 0,
        "concurrencyLimit": p.get("concurrencyLimit") or 0,
        "apiKey": api_key,
    }
    client.post("providers/update", body)
    emit(args, {"key": args.key, "apiKeySet": True}, lambda d: print("已更新供应商 %s 的 API Key" % args.key))


def cmd_providers_test(args):
    data = client_for(args).post("providers/test", {"providerKey": args.key})
    emit(args, data)


def cmd_providers_toggle(args):
    client_for(args).post("providers/toggle", {"providerKey": args.key, "enabled": on_off(args.state)})
    emit(args, {"providerKey": args.key, "enabled": on_off(args.state)}, lambda d: print("已%s供应商 %s" % ("启用" if d["enabled"] else "停用", args.key)))


def cmd_models_list(args):
    data = client_for(args).post("models/list")
    rows = ((data.get("catalog") or {}).get("models")) or []
    if args.kind:
        rows = [m for m in rows if m.get("kind") == args.kind]
    emit(
        args,
        rows,
        lambda d: table(
            d, [("id", "ID"), ("name", "名称"), ("kind", "类型"), ("enabled", "启用"), ("providerKey", "供应商"), ("modelKey", "上游模型名")]
        ),
    )


def cmd_models_ping(args):
    data = client_for(args).post("model-health/ping", {"modelId": args.id})
    check = data.get("check") or {}

    def human(d):
        print("状态：%s  延迟：%s ms  HTTP：%s" % (check.get("status"), check.get("latencyMs"), check.get("httpStatus")))
        if check.get("errorMessage"):
            print("错误：%s" % check["errorMessage"])

    emit(args, data, human)


def cmd_models_toggle(args):
    client_for(args).post("models/toggle", {"modelId": args.id, "enabled": on_off(args.state)})
    emit(args, {"id": args.id, "enabled": on_off(args.state)}, lambda d: print("已%s模型 %s" % ("启用" if d["enabled"] else "停用", args.id)))


def cmd_models_endpoints(args):
    catalog = client_for(args).post("models/list").get("catalog") or {}
    rows = catalog.get("endpoints") or []
    if args.kind:
        rows = [e for e in rows if (e.get("kind") or "").startswith(args.kind)]
    emit(
        args,
        rows,
        lambda d: table(d, [("key", "endpointKey"), ("name", "名称"), ("kind", "类型"), ("protocol", "协议"), ("path", "路径")]),
    )


def cmd_models_presets(args):
    catalog = client_for(args).post("models/list").get("catalog") or {}
    data = {"imagePresets": catalog.get("imagePresets") or [], "videoPresets": catalog.get("videoPresets") or []}

    def human(d):
        print("== 图片预设 ==")
        table(d["imagePresets"], [("key", "key"), ("label", "名称"), ("modelKey", "modelKey"), ("formTemplate", "表单模板")])
        print("\n== 视频预设 ==")
        table(d["videoPresets"], [("key", "key"), ("label", "名称"), ("endpointKey", "endpointKey"), ("modelKey", "modelKey")])

    emit(args, data, human)


def cmd_models_capabilities(args):
    emit(args, client_for(args).post("models/protocol-capabilities"))


def cmd_health(args):
    data = client_for(args).post("model-health/list", {"rangeHours": args.hours})
    emit(
        args,
        data,
        lambda d: table(
            [dict(r, successRate="%.0f%%" % (r["successRate"] * 100) if isinstance(r.get("successRate"), (int, float)) else "") for r in d.get("rows") or []],
            [("modelName", "模型"), ("providerKey", "供应商"), ("enabled", "启用"), ("lastStatus", "最近状态"),
             ("successRate", "成功率"), ("lastLatencyMs", "延迟ms"), ("lastCheckedAt", "最近探测")],
        ),
    )


def cmd_packages_list(args):
    data = client_for(args).post("packages/list")
    rows = data.get("packages") or []
    emit(
        args,
        rows,
        lambda d: table(d, [("id", "ID"), ("name", "名称"), ("kind", "类型"), ("credits", "积分"), ("priceCents", "价格(分)"), ("enabled", "上架")]),
    )


# ---------------------------------------------------------------------------
# Commands: content (articles, FAQ)
# ---------------------------------------------------------------------------


def read_text_file(path):
    try:
        with open(path, "r", encoding="utf-8") as f:
            return f.read()
    except OSError as e:
        raise CLIError("读不了 %s：%s" % (path, e), exit_code=EXIT_USAGE)


def fail_on_parse_errors(res):
    for w in res.get("warnings") or []:
        warn(w)
    errors = res.get("errors") or []
    if errors:
        raise CLIError("解析不通过：" + "；".join(errors), code="cli.parseFailed", exit_code=EXIT_USAGE, params={"errors": errors})


def cmd_content_publish(args):
    client = client_for(args)
    res = client.post("content/parse", {"markdown": read_text_file(args.file)})
    fail_on_parse_errors(res)
    article = res.get("article") or {}
    existing = res.get("existingId")
    if existing:
        confirm(args, "覆盖已有文章 %s/%s（%s）" % (article.get("kind"), article.get("slug"), article.get("locale")))
    body = dict(article, enabled=not args.disabled, overwrite=bool(existing))
    saved = client.post("content/upsert", body)
    emit(
        args,
        {"article": {k: article.get(k) for k in ("kind", "slug", "locale", "title")}, "updated": bool(existing), "enabled": not args.disabled, "result": saved},
        lambda d: print(
            "已%s文章「%s」（%s/%s，%s）"
            % ("更新" if existing else "保存", article.get("title"), article.get("kind"), article.get("slug"), article.get("locale"))
            + ("，未上线" if args.disabled else "，已上线")
        ),
    )


def cmd_faq_import(args):
    client = client_for(args)
    body = {"markdown": read_text_file(args.file)}
    if args.locale:
        body["locale"] = args.locale
    preview = client.post("faq/parse", body)
    fail_on_parse_errors(preview)
    cats = preview.get("categories") or []
    entries = preview.get("entries") or []
    summary = {
        "locale": preview.get("locale"),
        "categories": len(cats),
        "entries": len(entries),
        "updates": sum(1 for e in entries if e.get("exists")) + sum(1 for c in cats if c.get("exists")),
    }
    if args.dry_run:
        emit(args, {"preview": summary, "imported": False}, lambda d: print("将导入 %(categories)d 个分类、%(entries)d 个问题（其中 %(updates)d 项覆盖已有），--dry-run 未保存" % summary))
        return
    res = client.post("faq/import", body)
    emit(args, {"preview": summary, "imported": True, "result": res}, lambda d: print("已导入：%s" % dumps(res)))


# ---------------------------------------------------------------------------
# Commands: users, tickets
# ---------------------------------------------------------------------------


def cmd_users_find(args):
    data = client_for(args).post("users/list", {"keyword": args.keyword, "page": 1, "pageSize": args.limit})
    rows = data.get("users") or []
    emit(args, rows, lambda d: table(d, [("id", "ID"), ("name", "名称"), ("email", "邮箱"), ("phone", "手机"), ("role", "角色"), ("creditBalance", "积分")]))


def resolve_user_id(client, ref):
    if "@" not in ref and not ref.isdigit():
        return ref
    data = client.post("users/list", {"keyword": ref, "page": 1, "pageSize": 20})
    users = data.get("users") or []
    exact = [u for u in users if ref in (u.get("email"), u.get("phone"), u.get("id"))]
    if len(exact) == 1:
        return exact[0]["id"]
    if not exact and len(users) == 1:
        return users[0]["id"]
    raise CLIError("「%s」匹配到 %d 个用户，请改用用户 ID（users find 查看）" % (ref, len(exact) or len(users)), exit_code=EXIT_USAGE)


def cmd_users_grant(args):
    if args.amount <= 0:
        raise CLIError("积分要大于 0", exit_code=EXIT_USAGE)
    client = client_for(args)
    user_id = resolve_user_id(client, args.user)
    confirm_route(args, client, "users/grant-credits", "给用户 %s 发放 %d 积分" % (args.user, args.amount))
    body = {"userId": user_id, "credits": args.amount}
    if args.expires:
        body["expiresAt"] = to_rfc3339(args.expires)
    if args.note:
        body["note"] = args.note
    data = client.post("users/grant-credits", body)
    emit(args, data, lambda d: print("已发放 %d 积分，当前余额 %s" % (args.amount, d.get("balance"))))


def to_rfc3339(text):
    text = text.strip()
    try:
        if len(text) == 10:
            d = _dt.datetime.strptime(text, "%Y-%m-%d")
            return d.replace(hour=23, minute=59, second=59).astimezone().isoformat()
        return _dt.datetime.fromisoformat(text).astimezone().isoformat()
    except ValueError:
        raise CLIError("日期格式应为 2026-12-31 或 ISO 8601：%s" % text, exit_code=EXIT_USAGE)


def cmd_tickets_list(args):
    body = {"page": 1, "pageSize": args.limit}
    if args.status:
        body["status"] = args.status
    data = client_for(args).post("tickets/list", body)

    def human(d):
        print("待处理 %s · 等用户回复 %s · 已解决 %s · 未读 %s" % tuple((d.get("counts") or {}).get(k, 0) for k in ("open", "waitingUser", "resolved", "unread")))
        rows = [dict(t, userName=(t.get("user") or {}).get("name")) for t in d.get("tickets") or []]
        table(rows, [("id", "ID"), ("status", "状态"), ("category", "分类"), ("subject", "标题"), ("userName", "用户"), ("lastMessageAt", "最近消息")])

    emit(args, data, human)


# ---------------------------------------------------------------------------
# Commands: ops & logs
# ---------------------------------------------------------------------------


def cmd_ops_snapshot(args):
    emit(args, client_for(args).post("ops/snapshot"))


def cmd_ops_history(args):
    emit(args, client_for(args).post("ops/history", {"range": args.range}))


def parse_since(text):
    units = {"m": 60, "h": 3600, "d": 86400}
    text = text.strip().lower()
    if len(text) < 2 or text[-1] not in units or not text[:-1].isdigit():
        raise CLIError("--since 写成 30m / 6h / 7d：%s" % text, exit_code=EXIT_USAGE)
    since = _dt.datetime.now(_dt.timezone.utc) - _dt.timedelta(seconds=int(text[:-1]) * units[text[-1]])
    return since.isoformat().replace("+00:00", "Z")


def cmd_logs_query(args):
    body = {"limit": args.limit}
    if args.level:
        body["minLevel"] = args.level.upper()
    if args.since:
        body["from"] = parse_since(args.since)
    if args.keyword:
        body["keyword"] = args.keyword
    data = client_for(args).post("system-logs/query", body)

    def human(d):
        rows = d.get("rows") or []
        if not rows:
            print("（没有匹配的日志）")
        for r in rows:
            print("%s %-5s %s %s" % (r.get("time"), r.get("level"), r.get("mod") or "-", r.get("msg")))

    emit(args, data, human)


# ---------------------------------------------------------------------------
# help --json and the parser
# ---------------------------------------------------------------------------


def command_tree(parser, prefix=""):
    out = []
    for action in parser._actions:
        if isinstance(action, argparse._SubParsersAction):
            for name, sub in action.choices.items():
                full = (prefix + " " + name).strip()
                children = command_tree(sub, full)
                if children:
                    out.extend(children)
                else:
                    out.append({"command": full, "help": sub.description or ""})
    return out


def build_parser():
    def common_options(defaults):
        c = argparse.ArgumentParser(add_help=False)
        # Sub-commands repeat the global options with SUPPRESS defaults, so
        # `draw_admin.py --json whoami` and `draw_admin.py whoami --json` both work.
        d = (lambda v: v) if defaults else (lambda v: argparse.SUPPRESS)
        c.add_argument("--profile", default=d(None), help="配置档名（默认用 login 时保存的默认档）")
        c.add_argument("--json", action="store_true", default=d(False), help="输出机器可读的 JSON")
        c.add_argument("--yes", action="store_true", default=d(False), help="跳过需确认操作的确认（先征得运营方同意）")
        c.add_argument("--timeout", type=int, default=d(60), help="单次请求超时秒数")
        return c

    common = common_options(False)
    p = argparse.ArgumentParser(prog="draw_admin.py", description="站点运维 CLI（draw-site-admin）", parents=[common_options(True)])
    p.add_argument("--version", action="version", version=VERSION)
    sub = p.add_subparsers(dest="cmd", metavar="<命令>")

    def add(parent, name, fn, desc, **kw):
        sp = parent.add_parser(name, parents=[common], description=desc, help=desc, **kw)
        sp.set_defaults(fn=fn)
        return sp

    def group(name, desc):
        g = sub.add_parser(name, description=desc, help=desc)
        return g.add_subparsers(dest="sub", metavar="<子命令>")

    sp = add(sub, "login", cmd_login, "保存站点地址与运维密钥（密钥隐藏输入）")
    sp.add_argument("--base-url", help="站点地址，如 https://your-site.com")
    sp.add_argument("--key-stdin", action="store_true", help="从标准输入读一行密钥")
    add(sub, "logout", cmd_logout, "删除本机保存的配置档")
    add(sub, "profiles", cmd_profiles, "列出本机的配置档")
    sp = add(sub, "use", cmd_use, "切换默认配置档")
    sp.add_argument("name")
    add(sub, "whoami", cmd_whoami, "当前站点、签发人、密钥权限与版本")
    add(sub, "doctor", cmd_doctor, "站点体检：还缺哪些配置、怎么补")
    sp = add(sub, "routes", cmd_routes, "列出全部后台接口及所需权限")
    sp.add_argument("--scope", choices=["read", "config", "commerce", "users", "danger", "never"])
    sp = add(sub, "describe", cmd_describe, "查看一个后台接口的说明、权限档和请求体字段")
    sp.add_argument("path", help="接口路径（/api/admin/ 之后的部分）")
    sp = add(sub, "call", cmd_call, "调用任意后台接口，如 call models/list")
    sp.add_argument("path", help="接口路径（/api/admin/ 之后的部分）")
    sp.add_argument("--data", help="JSON 请求体")
    sp.add_argument("--data-file", help="从文件读 JSON 请求体")
    sp.add_argument("--file", help="上传文件（multipart，--data 的字段作为同级表单项）")

    st = group("settings", "系统设置")
    sp = add(st, "list", cmd_settings_list, "列出设置（密钥类只显示是否已设置）")
    sp.add_argument("--group", help="只看某一组，如 storage / mail / payment")
    sp = add(st, "get", cmd_settings_get, "查看设置项的值与说明")
    sp.add_argument("keys", nargs="+")
    sp = add(st, "set", cmd_settings_set, "修改设置：键=值 …；密钥类用 --secret / --from-env / --from-file")
    sp.add_argument("assignments", nargs="+", metavar="键=值")
    sp.add_argument("--secret", action="store_true", help="在终端隐藏输入这个键的值")
    sp.add_argument("--from-env", metavar="变量名", help="从环境变量读值")
    sp.add_argument("--from-file", metavar="文件", help="从文件读值（长文本、SVG Logo 等）")
    sp.add_argument("--dry-run", action="store_true", help="只显示改动，不保存")
    sp = add(st, "test-mail", cmd_settings_test_mail, "发一封测试邮件")
    sp.add_argument("to")
    add(st, "test-storage", cmd_settings_test_storage, "对象存储读写测试")
    sp = add(st, "upload-favicon", cmd_settings_upload_favicon, "上传站点图标并写入设置")
    sp.add_argument("file")
    sp = add(st, "upload-share-image", cmd_settings_upload_share_image, "上传分享图并写入设置")
    sp.add_argument("file")

    pv = group("providers", "AI 供应商")
    add(pv, "list", cmd_providers_list, "供应商列表")
    sp = add(pv, "create", cmd_providers_create, "新建供应商，标识由站点生成并打印出来；API Key 用 --secret 或 --from-env，不写在命令行")
    sp.add_argument("--name", required=True)
    sp.add_argument("--base-url", required=True, help="上游 Base URL（必填，没有默认值）")
    sp.add_argument("--description")
    sp.add_argument("--concurrency", type=int, help="并发上限，0 为不限")
    sp.add_argument("--secret", action="store_true", help="在终端隐藏输入 API Key")
    sp.add_argument("--from-env", metavar="变量名", help="从环境变量读 API Key")
    sp = add(pv, "set-key", cmd_providers_set_key, "更换供应商的 API Key")
    sp.add_argument("key")
    sp.add_argument("--secret", action="store_true", help="在终端隐藏输入 API Key")
    sp.add_argument("--from-env", metavar="变量名", help="从环境变量读 API Key")
    sp = add(pv, "test", cmd_providers_test, "测试供应商连通性")
    sp.add_argument("key")
    sp = add(pv, "toggle", cmd_providers_toggle, "启用或停用供应商")
    sp.add_argument("key")
    sp.add_argument("state", metavar="on|off")

    md = group("models", "模型")
    sp = add(md, "list", cmd_models_list, "模型列表")
    sp.add_argument("--kind", help="image / video / text …")
    sp = add(md, "ping", cmd_models_ping, "立即探测一个模型")
    sp.add_argument("id")
    sp = add(md, "toggle", cmd_models_toggle, "启用或停用模型")
    sp.add_argument("id")
    sp.add_argument("state", metavar="on|off")
    sp = add(md, "endpoints", cmd_models_endpoints, "可选的端点家族（新建模型时填 endpointKey）")
    sp.add_argument("--kind", help="image / video / text")
    add(md, "presets", cmd_models_presets, "内置的模型预设（常见模型的推荐配置）")
    add(md, "capabilities", cmd_models_capabilities, "各端点家族支持的参数")

    sp = add(sub, "health", cmd_health, "模型健康")
    sp.add_argument("--hours", type=int, default=24)

    pk = group("packages", "积分套餐")
    add(pk, "list", cmd_packages_list, "套餐列表")

    ct = group("content", "文章")
    sp = add(ct, "publish", cmd_content_publish, "解析并发布一篇带 frontmatter 的 Markdown 文章；覆盖已有文章需确认")
    sp.add_argument("file")
    sp.add_argument("--disabled", action="store_true", help="保存但不上线")
    fq = group("faq", "帮助中心")
    sp = add(fq, "import", cmd_faq_import, "导入一个 FAQ Markdown 文件")
    sp.add_argument("file")
    sp.add_argument("--locale", choices=["zh", "en"], help="覆盖文件 frontmatter 里的 locale")
    sp.add_argument("--dry-run", action="store_true", help="只预览，不保存")

    us = group("users", "用户")
    sp = add(us, "find", cmd_users_find, "按邮箱、手机、名称或 ID 查用户")
    sp.add_argument("keyword")
    sp.add_argument("--limit", type=int, default=20)
    sp = add(us, "grant-credits", cmd_users_grant, "给用户发放积分（需确认）")
    sp.add_argument("user", help="用户 ID、邮箱或手机")
    sp.add_argument("amount", type=int)
    sp.add_argument("--expires", help="过期日期，如 2026-12-31；不填为永久")
    sp.add_argument("--note", help="备注")

    tk = group("tickets", "工单")
    sp = add(tk, "list", cmd_tickets_list, "工单列表")
    sp.add_argument("--status", help="open / waiting_user / resolved / closed")
    sp.add_argument("--limit", type=int, default=20)

    op = group("ops", "运维监控")
    add(op, "snapshot", cmd_ops_snapshot, "当前快照")
    sp = add(op, "history", cmd_ops_history, "历史曲线")
    sp.add_argument("--range", choices=["1h", "6h", "24h"], default="24h")

    lg = group("logs", "系统日志")
    sp = add(lg, "query", cmd_logs_query, "查询系统日志")
    sp.add_argument("--level", choices=["warn", "error"], help="最低级别")
    sp.add_argument("--since", help="最近多久，如 30m / 6h / 7d")
    sp.add_argument("--keyword")
    sp.add_argument("--limit", type=int, default=50)

    sp = add(sub, "help", None, "显示帮助；--json 输出全部命令")
    return p


def main(argv=None):
    parser = build_parser()
    args = parser.parse_args(argv)
    if args.cmd == "help" or args.cmd is None:
        if getattr(args, "json", False):
            print(dumps({"ok": True, "data": {"version": VERSION, "commands": command_tree(parser)}}))
        else:
            parser.print_help()
        return EXIT_OK
    fn = getattr(args, "fn", None)
    if fn is None:
        parser.parse_args([args.cmd, "--help"])  # prints the group's sub-commands and exits
        return EXIT_USAGE
    try:
        fn(args)
        return EXIT_OK
    except CLIError as e:
        if args.json:
            print(dumps({"ok": False, "error": {"code": e.code, "params": e.params, "message": e.message}}))
        else:
            print("错误：%s（%s）" % (e.message, e.code), file=sys.stderr)
        return e.exit_code
    except KeyboardInterrupt:
        print("已中断", file=sys.stderr)
        return EXIT_FAIL


if __name__ == "__main__":
    sys.exit(main())

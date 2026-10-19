# 内容运营

站点的对外内容只有中文和英文两份（`zh` / `en`）：繁体中文读者看中文版，日语、韩语读者看英文版。不要建其它语言的内容。

所有对外文字：地道的中文，不写「进行」「为您」「基于」这类垫词，不写复述标题、介绍页面、罗列功能的句子，一件事说一次。站名、活动时间、价格、联系方式这些事实一律问运营方。写好先给运营方看，同意后再发布。

## 公告

```bash
draw_admin.py call announcements/list
draw_admin.py call announcements/create --data '{"title":"国庆期间充值八折","content":"10 月 1 日至 7 日，全部积分套餐八折。\n\n![活动海报](https://img.example.com/poster.jpg)","category":"event","coverUrl":"https://img.example.com/cover.jpg","actionLabel":"去充值","actionUrl":"/billing","enabled":true,"popup":false,"pinned":true}'
draw_admin.py call announcements/toggle --data '{"id":"…","enabled":false}'
```

- `category`：`event`（活动）、`update`（更新）、`maintenance`（维护）、`notice`（通知，默认）。
- `content` 是 Markdown，5000 字以内；配图用 `![说明](https://图片直链)`。
- `coverUrl` 可选，填 http(s) 图片直链，按 12:5 横图裁切显示；直链不经过本站存储，图床失效就不显示。
- `actionLabel` + `actionUrl` 可选，要么都填要么都空：站内页面写 `/billing` 这样的路径，外部页面写完整的 https 地址。
- `popup` 会在用户打开站点时弹窗，少用；`pinned` 置顶。下线用 `toggle`，不要删。

运营方想自己在后台贴：把公告写成 Markdown，开头用 frontmatter 写 `title`、`category`、`pinned`、`popup`、`cover`、`button`、`link`（不需要的删掉），交给运营方在「公告管理 → 粘贴导入」里贴入，确认后创建。

## 文章（使用指南、场景方案、对比、博客）

文章是带 frontmatter 的 Markdown。写法和 frontmatter 格式见 seo-article 技能：https://raw.githubusercontent.com/arxuan09/draw-deploy/main/skills/seo-article/SKILL.md 。

frontmatter 里写 `kind`（`guides` / `scene` / `compare` / `blog`）、`slug`、`locale`、`title`、`published_at` 等。发布：

```bash
draw_admin.py content publish article.md               # 解析 → 有错误就停 → 保存并上线
draw_admin.py content publish article.md --disabled    # 只保存，不上线
draw_admin.py call content/list --data '{"locale":"zh"}'
```

解析时的提示（缺封面、正文没带站名等）会打印出来，转告运营方。同一 `kind + slug + locale` 已有文章时算覆盖，CLI 会要求确认。下线用 `call content/toggle --data '{"id":"…","enabled":false}'`。

## FAQ（帮助中心）

一个 Markdown 文件装一整个帮助中心板块，格式：

```markdown
---
locale: zh
---

# 新手入门 {#getting-started}

可选：分类描述。

## 怎么注册账号？ {#how-to-register}

> 可选：紧跟标题的引用块是列表页上的摘要。

正文 Markdown，小标题用 ### 及以下。
```

`#` 是分类、`##` 是问题，`{#…}` 是 slug。按 `slug + locale` 匹配：已有的更新，没有的新建，文件里没提到的不动。

```bash
draw_admin.py faq import faq.md --dry-run     # 预览：几个分类、几个问题、覆盖几项
draw_admin.py faq import faq.md
```

## 用户协议、隐私政策

在系统设置里，见 `references/setup.md` 的 legal 一节。

## 联系我们

```bash
draw_admin.py call contact-page/get
draw_admin.py call contact-page/save --data-file contact.json
```

先 `get` 拿到完整结构，改完整份保存。二维码图片先 `call contact-page/upload-image --file qr.png` 拿到地址再填进去。

## 批量推送

`broadcasts/*` 会向全站用户发站内信或邮件，属于 `danger` 档，且一旦发出无法撤回。只有运营方明确要求时才做：先把标题、正文、发送对象、预计人数给他确认。

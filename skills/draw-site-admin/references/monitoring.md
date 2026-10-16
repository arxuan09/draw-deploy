# 巡检与日报

巡检只需要 `read` 档。定时跑巡检的 Agent（服务器上的 cron、云端定时任务）建议单独签一把只读密钥，并限制来源地址；用环境变量 `DRAW_BASE_URL` + `DRAW_ADMIN_KEY` 传入，不写配置文件。

## 看什么

```bash
draw_admin.py ops snapshot --json                         # 当前：应用、HTTP、队列、MySQL、Redis、主机、缓存、业务
draw_admin.py ops history --range 24h --json              # 24 小时曲线
draw_admin.py health --json                               # 各模型最近成功率、延迟、状态
draw_admin.py logs query --level error --since 24h --json
draw_admin.py tickets list --status open
draw_admin.py call dashboard/read                         # 注册、出图、收入
draw_admin.py doctor --json                               # 配置有没有被改坏
```

`ops snapshot` 的 `health` 字段按组（app、http、mysql、redis、host、cache…）给出 `status`（`ok` / `warn` / `crit`）和每项检查的 `label`、`value`。阈值是站点内置的，`warn` / `crit` 就是异常，不用自己另定标准。

## 什么算异常

| 信号 | 说明 |
| --- | --- |
| `health` 里任一组 `crit` | 立即报告，附上检查项的 `label` 和 `value` |
| 某个启用的模型成功率明显低于其它模型，或 `lastStatus` 为 `down` | 上游出问题，查 `logs query --keyword <模型名>`；有备用模型的话建议运营方临时停用这个 |
| 错误日志里同一条消息反复出现 | 按消息归并计数，报最多的几条 |
| 待处理工单超过一天没人回 | 列出标题和用户 |
| `doctor` 的必须项从 OK 变成缺失 | 配置被改坏或第三方失效（证书、密钥过期） |

只报异常和需要运营方动手的事，正常的指标一句话带过。

## 日报格式

```
【站点名 · 2026-10-16 日报】
总体：正常 / 有 N 项需要处理
需要处理：
1. GPT Image 2 最近 24 小时成功率 71%（其它模型 98%），错误多为上游 429，建议检查该供应商余额。
2. 2 个工单超过 24 小时未回复：「出图一直失败」「发票开错了」。
数据：新注册 132，出图 4,210 张，收入 ¥1,280。
```

数字从接口原样取，不要估算；接口没给的数据就不写。

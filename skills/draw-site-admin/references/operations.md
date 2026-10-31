# 日常运营

这一类操作大多动钱或动用户，需要 `commerce` / `users` 档，CLI 会要求确认（退出码 8）。每次都先把对象、数量、金额列给运营方，同意后再加 `--yes`。

## 查用户

```bash
draw_admin.py users find someone@example.com
draw_admin.py call users/get --data '{"userId":"…"}'      # 详情：积分批次、订单、登录方式
```

## 发放 / 扣除积分

```bash
draw_admin.py users grant-credits someone@example.com 100 --expires 2026-12-31 --note "活动补偿"
draw_admin.py call users/deduct-credits --data '{"userId":"…","credits":100,"note":"误发回收"}' --yes
```

`--expires` 不填为永久。批量发放先 `users find` 逐个确认用户，把名单和总积分给运营方看，再逐个执行；任何一个失败就停下报告，不要跳过。

## 订单与退款

```bash
draw_admin.py call orders/list --data '{"page":1,"pageSize":20,"status":"pending","keyword":""}'
draw_admin.py call refunds/list --data '{}'
draw_admin.py call refunds/review --data '{"refundId":"…","approve":true}' --yes
draw_admin.py call refunds/review --data '{"refundId":"…","approve":false,"rejectReason":"已超过退款期限"}' --yes
draw_admin.py call refunds/complete --data '{"refundId":"…","refundTradeNo":"支付平台的退款单号"}' --yes
```

Stripe、Waffo、支付宝官方、微信支付官方的订单用 `refunds/to-source` 原路退回（微信和 Waffo 的退款先受理、稍后到账，站点会自动跟进，失败时给管理员发告警，之后对同一张申请再调一次 `refunds/to-source` 就是重试）；易支付等其它通道要运营方先在支付平台后台退款，再用 `refunds/complete` 填退款单号。`orders/mark-paid`（手动标记已支付并发积分）只在运营方确认钱已经到账、但回调丢了时用，要带支付平台的交易号。

## 卡密与优惠券

```bash
draw_admin.py call redeem-codes/create --data '{"count":50,"credits":100,"validityDays":30,"note":"十月活动","channel":"小红书"}' --yes
draw_admin.py call coupons/list
```

生成卡密等于发钱，数量和面额务必和运营方核对。

## 工单

```bash
draw_admin.py tickets list --status open
draw_admin.py call tickets/messages --data '{"ticketId":"…"}'
draw_admin.py call tickets/reply --data '{"ticketId":"…","body":"已为你补发 20 积分，请刷新查看。","resolve":true}'
```

回复用户的话要先给运营方看。能直接解决的（补积分、解释用法）连同操作一起提给运营方；涉及退款、封号的交给运营方决定。

## 用户账号

```bash
draw_admin.py call users/reset-password --data '{"userId":"…","newPassword":"…"}' --yes
draw_admin.py call users/unbind --data '{"userId":"…","binding":"wechat"}' --yes      # wechat / qq / phone / password
```

管理员账号一律碰不了（`errors.apiKey.adminTarget`），请运营方本人处理。

## 恢复用户的画布

用户说画布内容没了、被清空、少了一大半时，先别让他继续在那个画布上操作。站点会在保存时自动留历史版本：一次删掉一半以上（原来至少 5 个对象）、清空、「以当前页面为准」覆盖时都会留，平时编辑每 30 分钟最多留一个。每个项目平时编辑的版本最多 12 个、删改类的重要版本最多 10 个，各挤各的，保留 30 天。

```bash
draw_admin.py users find someone@example.com
draw_admin.py call project-revisions/projects --data '{"userId":"…"}'         # 最近更新的项目和各自的历史版本数
draw_admin.py call project-revisions/list --data '{"projectId":"…"}'           # 每个版本的时间、对象数、原因
draw_admin.py call project-revisions/restore --data '{"projectId":"…","revisionId":"…"}' --yes
```

用户给的链接里 `projectId=` 后面就是项目 ID。管理员账号的画布密钥看不了也恢复不了（`errors.apiKey.adminTarget`），请运营方在网页后台处理。把候选版本（时间、对象数、原因）列给运营方，由他和用户确认要回到哪一个。恢复前当前内容会先存成一个 `before-restore` 版本，恢复错了再对它 `restore` 一次就回来了。恢复后请用户刷新页面；开着的旧页面再保存会弹冲突提示，让他选「加载最新内容」，不要选「以当前页面为准」。

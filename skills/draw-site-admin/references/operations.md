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

# 支付通道

**支付配置只能运营方本人在网页后台填。** 运维密钥写不了系统设置「支付」组的任何一项（站点币种、Stripe / Waffo / 易支付的账户、密钥和 Webhook 设置、Waffo 环境和基础商品、所有支付开关），空着也不行（`errors.apiKey.paymentSettingLocked`）；支付通道和自定义支付方式也不能新建、修改、启停、排序或删除（`errors.apiKey.routeDenied`）。钱收到谁的账户、开哪个通道，必须是运营方自己定的。

所以这一步你做的是：告诉运营方要准备什么、去后台哪一页填、对方平台的 Webhook 怎么配；他填完后你来验证。不要让他把支付密钥贴给你，也不要试别的办法绕过。

## 先定币种

`site_currency`：`cny`（人民币站）或 `usd`（美元站），一站一个币种，在「系统设置 → 支付」。**上线前定好，之后不要改**——已有套餐价格和历史订单不会换算。美元站不能用支付宝 / 微信。

```bash
draw_admin.py settings get site_currency
```

人民币站一般用易支付或支付宝 / 微信支付官方；美元站用 Stripe；Waffo Pancake 两种站都能开（人民币站只有微信一次性付款）。问运营方已经开通了哪家，没开通的别替他决定。

下面的「回调地址」里，`https://站点` 指 `site_public_url`，先确认它填对了。

## 易支付、支付宝 / 微信支付官方（支付通道页）

在网页后台「系统 → 支付通道」里配置，可以同时挂多个；同一种支付方式有几个通道时，用户结算时自己选线路。易支付网关支持的其它方式（QQ 钱包、云闪付等）在同一页的「自定义支付方式」里加：填易支付的调用值和显示名，再在易支付通道里勾上。

请运营方准备并自己填：易支付要网关地址、商户 PID、商户密钥；支付宝要 AppID、应用私钥、支付宝公钥（或三张证书）；微信支付要商户号、AppID、API 证书序列号、商户私钥、APIv3 密钥、微信支付公钥和公钥 ID。回调地址站点下单时自动带上，不用在支付平台配；微信支付要在商户平台配 H5 支付域名和 JSAPI 支付授权目录（后台页面底部有，可一键复制）。

他填好后你来测：

```bash
draw_admin.py call payment-channels/list
draw_admin.py call payment-channels/test --data '{"id":"通道 id"}'
```

再请运营方用最小金额套餐实际付一笔，确认订单变成已支付、积分到账。

## Stripe

请运营方准备：Publishable Key（`pk_…`）、Secret Key（`sk_…`）、Webhook Signing Secret（`whsec_…`）；测试模式还是正式模式（`pk_test` / `pk_live` 要和 `sk_` 一致）。

请他在 Stripe 后台「Developers → Webhooks」新建端点：

- 地址：`https://站点/api/billing/orders/stripe-webhook`
- 事件：`checkout.session.completed`、`invoice.paid`、`invoice.payment_failed`、`customer.subscription.updated`、`customer.subscription.deleted`、`charge.dispute.created`、`charge.refunded`

然后在网页后台「系统设置 → 支付」填三个密钥、打开「Stripe 开关」并保存。填完你来验证：

```bash
draw_admin.py call settings/stripe-check
```

`stripe-check` 会去 Stripe 核对密钥、Webhook 端点和事件订阅，列出问题；有问题告诉运营方改哪一项。全部通过后请运营方实际付一笔。自动续费订阅套餐（`billingInterval` 为 `month` / `year`）只走 Stripe 和 Waffo。

## Waffo Pancake

请运营方准备：环境（`test` / `prod`）、商户 ID、店铺 ID、该环境的 API 私钥；Webhook 公钥可选。请他在网页后台「系统设置 → 支付」填好这几项并保存（先不开「Waffo Pancake 开关」）。

填完后基础商品可以你来建（用的是运营方填的账户，在他的店铺里建）：

```bash
draw_admin.py call settings/waffo-create-products     # 在 Waffo 创建一次性 / 月付 / 年付基础商品，写回 waffo_product_*，再登记当前环境的 Webhook
```

每次调用都会新建一套商品。商品建好后它接着在当前环境（`waffo_environment`）登记 Webhook，结果在返回的 `result.webhook` 里。只缺 Webhook（商品已有）时别再建商品，单独登记：

```bash
draw_admin.py call settings/waffo-register-webhook    # 只登记当前环境的 Webhook，不建商品；结果在 webhook 里
```

两者的 Webhook 结果相同：`added`（新登记）、`updated`（地址已在，补上了缺的事件，运营方自己加的事件保留）、`present`（已齐全）、`failed`（`message` 写原因；建商品时商品照样已保存）。只看、只改地址正好是本站回调地址的那条，别的 Webhook 不动。登记要求站点公开地址是 https，且 API 私钥所属商户是店铺的 owner 或 admin；店铺最多 20 个 Webhook。

建好后请运营方在同一页打开「Waffo Pancake 开关」。

`failed` 时请运营方在 Waffo 后台 → Settings → Webhooks 手动添加：地址 `https://站点/api/billing/orders/waffo-webhook`，订阅 `order.completed`、`subscription.activated`、`subscription.payment_succeeded`、`subscription.renewed`、`subscription.recovered`、`subscription.past_due`、`subscription.canceling`、`subscription.uncanceled`、`subscription.canceled`、`refund.succeeded`、`refund.failed`（后台「原路退款」靠后两个确认到账）。测试和生产各一条。`doctor` 的 payment 一项会查当前环境有没有这条 Webhook、事件全不全，缺了就跑 `settings/waffo-register-webhook`。

测试环境任何人都能用 Waffo 公开的测试卡付款，上线前务必切到 `prod`：运营方在网页后台换成生产环境和生产私钥，之后你再跑一次 `settings/waffo-create-products`（建生产商品，同时登记生产环境的 Webhook）。人民币站的 Waffo 只有微信，单价和同一买家每月累计都封顶 ¥1,000，只能卖一次性套餐。

## 验证

```bash
draw_admin.py doctor          # payment 一项
draw_admin.py call orders/list --data '{"page":1,"pageSize":5}'
```

真实付款只能运营方自己做。付完看订单状态和用户积分；没到账先查 `draw_admin.py logs query --level warn --since 30m --keyword webhook`。

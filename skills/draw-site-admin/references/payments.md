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

请运营方准备：Publishable Key（`pk_…`）和 Secret Key（`sk_…`）；测试模式还是正式模式（`pk_test` / `pk_live` 要和 `sk_` 一致）。用受限密钥（`rk_…`）时要开的权限：Checkout Sessions、Subscriptions、Refunds、Customer portal、Webhook Endpoints 写；Invoices、Invoice Payments、Charges、Payment Intents、Customers、Disputes 读；**账户信息（Account）读，必须开**——本站靠它认出订阅在哪个 Stripe 账户上，读不到时找不到的订阅一律不当作已结束，只告警。

本站所有 Stripe 请求固定用 API 版本 `2026-10-28.endive`（随官方 Go SDK 锁定），不受 Stripe 账户默认版本影响。

请他在网页后台「系统设置 → 支付」填两个密钥、打开「Stripe 开关」并保存，再点 Webhook Secret 下面的「登记回调地址」：本站用已保存的密钥在 Stripe 建端点 `https://站点/api/billing/orders/stripe-webhook`（固定版本、事件齐全），签名密钥直接存进 `stripe_webhook_secret`、端点 ID 记在 `stripe_webhook_endpoint_id`，不经过浏览器。站点公开地址必须是 https（只有测试密钥可以用 localhost）。这一步会写支付设置，所以只能运营方在网页后台点，运维密钥调不了。

- 已保存的签名密钥所属的那个端点（登记时记下的 ID）在本站地址上：只补缺少的事件并启用。手动填的密钥没有记 ID，只认这个地址上唯一一个启用的端点。别的端点（包括停用的旧端点）一律不会被重新启用。
- 这个地址上有端点，但本站的签名密钥不是它的：请运营方在 Stripe 后台 Workbench → Webhooks 选中该端点，点「Click to reveal」复制密钥填进「Webhook Secret」（不用 Roll；只有怀疑密钥泄露时才 Roll）。
- 本站还没有这个地址的端点，但已存过签名密钥，或本站的端点在别的地址（换过站点地址）：按钮不会自己新建，会列出旧端点，请运营方确认后点「新建端点并替换」。
- 要对齐 API 版本、签名密钥填错了（`stripe-check` 报验签连续失败），也点「新建端点并替换」：本站新建端点、保存新密钥、停用旧端点（不删除）。旧端点上还在等待重试的事件随之丢失：其中付款和续费会被每 10 分钟的对账和每小时的订阅同步补上，但拒付和 `refund.failed`、`refund.updated` 补不回来，替换前后要到 Stripe 后台看一眼有没有未处理的拒付和失败退款。停用失败的旧端点会列在页面上，要手动停用。

不用按钮、手动建端点时：Stripe 后台 Workbench → Webhooks 新建端点，地址同上，API 版本选 `2026-10-28.endive`，勾选这些事件：

`checkout.session.completed`、`invoice.paid`、`invoice.payment_failed`、`customer.subscription.updated`、`customer.subscription.deleted`、`charge.dispute.created`、`charge.dispute.updated`、`charge.dispute.funds_withdrawn`、`charge.dispute.closed`、`charge.refunded`、`refund.failed`、`refund.updated`

然后点「Click to reveal」把签名密钥（`whsec_…`）填进「Webhook Secret」。

请运营方在 Stripe 后台再设这几项（本站代替不了）：

- Billing → Revenue recovery → Retries：开 Smart Retries；全部重试失败后选「取消订阅」。不要选「暂停」，也不要手动暂停 / 恢复订阅。本站不支持暂停。暂停的订阅不再发积分，用户看到「已暂停」，管理员收到告警；恢复时 Stripe 开的是按比例计费的账单，本站只记收款不发积分，要管理员手动处理。
- 不要把取消日期定在周期中间（按比例计费的账单同样要人工处理）；在本站取消就是到期取消（逾期或暂停时立即结束）。
- Settings → Customer emails：打开扣款失败邮件、需要验证（3DS）时的邮件和付款成功收据。续费要验证时只靠这封邮件提醒买家。
- 销售税：Stripe 直收时运营方是卖方，开不开 Stripe Tax 由他定。

填完你来验证：

```bash
draw_admin.py call settings/stripe-check
```

`stripe-check` 会去 Stripe 核对密钥、Webhook 端点、事件订阅和启用状态，并给出三个版本：本站固定的、端点的、账户默认的。`problems` 必须改，受限密钥没有 Account 读权限也算在里面。`notes` 里的版本不一致不影响收款，要对齐请运营方点「新建端点并替换」（端点建好后版本改不了）。`doctor` 的 payment 一项也会查这条端点。全部通过后请运营方实际付一笔。自动续费订阅套餐（`billingInterval` 为 `month` / `year`）只走 Stripe 和 Waffo。

本站对拒付和退款的处理，运营方要知道：拒付问询（Stripe 里状态以 `warning_` 开头）只通知管理员，订阅照常、积分不收回；升级为正式拒付后才停订阅、收回积分；结案时通知胜诉或败诉，胜诉不会自动恢复积分，要管理员手动补发。后台「原路退款」几天后在 Stripe 失败（`refund.failed`）或被取消（`refund.updated`）时，钱回到 Stripe 余额，本站通知管理员。要换其他方式退给买家，退好后在后台退款列表点「已另行退款」：Stripe 一般不再接受退到退款失败过的卡，「重试原路退款」多半会被拒（页面会显示 Stripe 的原话）。已收回的积分不退回。

## Waffo Pancake

请运营方准备：环境（`test` / `prod`）、商户 ID、店铺 ID、该环境的 API 私钥；Webhook 公钥可选。请他在网页后台「系统设置 → 支付」填好这几项并保存（先不开「Waffo Pancake 开关」）。

填完后基础商品可以你来建（用的是运营方填的账户，在他的店铺里建）：

```bash
draw_admin.py call settings/waffo-create-products     # 在 Waffo 创建一次性 / 月付 / 季付 / 年付基础商品，写回 waffo_product_*，再登记当前环境的 Webhook
```

每次调用都会新建一套商品。商品建好后它接着在当前环境（`waffo_environment`）登记 Webhook，结果在返回的 `result.webhook` 里。只缺 Webhook（商品已有）时别再建商品，单独登记：

```bash
draw_admin.py call settings/waffo-register-webhook    # 只登记当前环境的 Webhook，不建商品；结果在 webhook 里
```

两者的 Webhook 结果相同：`added`（新登记）、`updated`（地址已在，补上了缺的事件，运营方自己加的事件保留）、`present`（已齐全）、`failed`（`message` 写原因；建商品时商品照样已保存）。只看、只改地址正好是本站回调地址的那条，别的 Webhook 不动。登记要求站点公开地址是 https，且 API 私钥所属商户是店铺的 owner 或 admin；店铺最多 20 个 Webhook。

建好后请运营方在同一页打开「Waffo Pancake 开关」。

`failed` 时请运营方在 Waffo 后台 → Settings → Webhooks 手动添加：地址 `https://站点/api/billing/orders/waffo-webhook`，订阅 `order.completed`、`subscription.activated`、`subscription.payment_succeeded`、`subscription.renewed`、`subscription.recovered`、`subscription.past_due`、`subscription.canceling`、`subscription.uncanceled`、`subscription.canceled`、`subscription.plan_changed`、`subscription.plan_change_scheduled`、`subscription.plan_change_failed`（本站不做 Waffo 换套餐，收到只告警）、`refund.succeeded`、`refund.failed`（后台「原路退款」靠后两个确认到账）。测试和生产各一条。`doctor` 的 payment 一项会查当前环境有没有这条 Webhook、事件全不全，缺了就跑 `settings/waffo-register-webhook`。

测试环境任何人都能用 Waffo 公开的测试卡付款，上线前务必切到 `prod`：运营方在网页后台换成生产环境和生产私钥，之后你再跑一次 `settings/waffo-create-products`（建生产商品，同时登记生产环境的 Webhook）。人民币站的 Waffo 只有微信，单价和同一买家每月累计都封顶 ¥1,000，只能卖一次性套餐。

## 验证

```bash
draw_admin.py doctor          # payment 一项
draw_admin.py call orders/list --data '{"page":1,"pageSize":5}'
```

真实付款只能运营方自己做。付完看订单状态和用户积分；没到账先查 `draw_admin.py logs query --level warn --since 30m --keyword webhook`。

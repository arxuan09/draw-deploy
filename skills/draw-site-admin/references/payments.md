# 支付通道

## 先定币种

`site_currency`：`cny`（人民币站）或 `usd`（美元站），一站一个币种。**上线前定好，之后不要改**——已有套餐价格和历史订单不会换算。美元站不能用支付宝 / 微信。

```bash
draw_admin.py settings get site_currency
```

人民币站一般用易支付；美元站用 Stripe；Waffo Pancake 两种站都能开（人民币站只有微信一次性付款）。问运营方已经开通了哪家，没开通的别替他决定。

下面的「回调地址」里，`https://站点` 指 `site_public_url`，先确认它填对了。

## 易支付（epay）

问：易支付网关地址、商户 PID、商户密钥；开支付宝还是微信。

```bash
draw_admin.py settings set epay_base_url=https://pay.example.com epay_pid=1001 epay_enable_alipay=true epay_enable_wxpay=true
draw_admin.py settings set epay_key --secret
draw_admin.py settings set epay_enabled=true
```

异步通知地址由站点在下单时自动带上（`https://站点/api/billing/orders/notify`），不用在易支付后台配。请运营方用最小金额套餐实际付一笔，确认订单变成已支付、积分到账。

## Stripe

问：Publishable Key（`pk_…`）、Secret Key（`sk_…`）、Webhook Signing Secret（`whsec_…`）；用测试模式还是正式模式（`pk_test` / `pk_live` 要和 `sk_` 一致）。

请运营方在 Stripe 后台「Developers → Webhooks」新建端点：

- 地址：`https://站点/api/billing/orders/stripe-webhook`
- 事件：`checkout.session.completed`、`invoice.paid`、`invoice.payment_failed`、`customer.subscription.updated`、`customer.subscription.deleted`、`charge.dispute.created`、`charge.refunded`

然后：

```bash
draw_admin.py settings set stripe_publishable_key=pk_live_…
draw_admin.py settings set stripe_secret_key --secret
draw_admin.py settings set stripe_webhook_secret --secret
draw_admin.py settings set stripe_enabled=true
draw_admin.py call settings/stripe-check
```

`stripe-check` 会去 Stripe 核对密钥、Webhook 端点和事件订阅，列出问题。全部通过后请运营方实际付一笔。自动续费订阅套餐（`billingInterval` 为 `month` / `year`）只走 Stripe 和 Waffo。

## Waffo Pancake

问：环境（`test` / `prod`）、商户 ID、店铺 ID、该环境的 API 私钥；Webhook 公钥可选。

```bash
draw_admin.py settings set waffo_environment=test waffo_merchant_id=… waffo_store_id=…
draw_admin.py settings set waffo_private_key --from-file waffo_private.pem      # 私钥文件，运营方提供路径
draw_admin.py call settings/waffo-create-products                             # 在 Waffo 创建一次性 / 月付 / 年付基础商品，写回 waffo_product_*
draw_admin.py settings set waffo_enabled=true
```

请运营方在 Waffo 后台配置 Webhook：地址 `https://站点/api/billing/orders/waffo-webhook`，订阅 `order.completed`，以及 `subscription.activated`、`subscription.payment_succeeded`、`subscription.renewed`、`subscription.past_due`、`subscription.recovered`、`subscription.canceling`、`subscription.uncanceled`、`subscription.canceled`；`refund.succeeded` 可选。测试和生产各配一次。

测试环境任何人都能用 Waffo 公开的测试卡付款，上线前务必切到 `prod`（换成生产私钥、重新创建基础商品）。人民币站的 Waffo 只有微信，单价和同一买家每月累计都封顶 ¥1,000，只能卖一次性套餐。

## 验证

```bash
draw_admin.py doctor          # payment 一项
draw_admin.py call orders/list --data '{"page":1,"pageSize":5}'
```

真实付款只能运营方自己做。付完看订单状态和用户积分；没到账先查 `draw_admin.py logs query --level warn --since 30m --keyword webhook`。

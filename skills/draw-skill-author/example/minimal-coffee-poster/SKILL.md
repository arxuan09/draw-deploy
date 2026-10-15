---
format: draw-platform-skill/1
slug: minimal-coffee-poster
name: { zh: "极简咖啡新品海报", en: "Minimal Coffee Launch Poster" }
summary: { zh: "做极简风的咖啡新品海报:单杯饮品居中特写、大面积留白、细衬线英文标题配一行中文,竖版 3:4。用户提到咖啡新品海报、饮品上新、极简饮品海报时使用。", en: "Minimal coffee launch posters: one drink centred in close-up, lots of white space, a thin serif title, portrait 3:4. Use for new-drink posters, coffee launch visuals or minimal drink posters." }
placeholder: { zh: "给新品[饮品名]做一张极简海报,品牌色[颜色]…", en: "A minimal poster for our new [drink], brand colour [colour]…" }
category: design
keywords: [咖啡, 新品海报, 极简, 饮品]
references:
  - { file: layout.png, note: "借版式:标题在上三分之一、主体居中、底部一行小字;不借配色" }
---

# 极简咖啡新品海报

## 目标

为咖啡店做一张竖版新品海报:一杯饮品是唯一的主角,靠留白和克制的字体做出质感。

## 系统能力边界

| 要做的事 | 用什么 |
| --- | --- |
| 出图 | `generate_image`,`aspect_ratio: "3:4"`、`count: 1` |
| 用户给了饮品照片 | 出图时设 `use_reference_images: true`,饮品外观以照片为准 |
| 套用本技能的版式参考图 | 把参考图 URL 放进 `reference_image_urls`,排在用户照片之后 |
| 确认标题 | `ask`,给两三个候选 |
| 放大成品 | `edit_image`,`operation: "upscale"` |

做不到:直接排版可编辑的文字图层、替用户印刷。用户要可编辑文字时,出一张不带字的底图,建议用画布的文字工具排标题。

## 输入

- 饮品名称(必需);没有就问。
- 饮品照片、品牌色(可选)。

## 流程

1. 根据饮品名拟英文标题和一行中文副标,调用 `ask` 让用户选或改。
2. 按下面的要点写 prompt,调用 `generate_image` 出图。
3. 按「成图自检」检查,不合格说明原因并在用户同意后重出。

## 默认输出

`aspect_ratio: "3:4"`、`resolution: "2K"`、`count: 1`;用户当场指定的优先。

## 要点

- 单杯饮品居中,占画面高度约 40%,略俯视的特写。
- 背景是奶油白或浅灰的纯色,留白不少于画面一半。
- 英文标题用细衬线体,放在上三分之一;中文副标一行,字号约为标题一半。
- 用户给了品牌色时,只用在标题或一处小色块上。

## 参考图怎么用

`layout.png` 只借版式比例(标题位置、主体位置、底部小字),不借它的颜色和线条。

## 禁止

- 不出现多杯饮品、甜点、人物等第二主体。
- 不加价格、促销语、二维码,除非用户明确要求。
- 不堆叠装饰元素(花瓣、咖啡豆撒满画面)。

## 和相近技能的区别

适合单品上新、需要高级感的海报;要做菜单、多品组合或促销海报时,不要用本技能。

## 成图自检

- 标题文字是否与确认的一致,有没有错字或乱码。
- 是否只有一个主体,留白是否足够。
- 比例是否为 3:4。

## 示例

用户:「下周上燕麦拿铁,帮我做张海报」→ 先给标题候选「Oat Latte / 燕麦拿铁 新品上市」,确认后出一张 3:4 极简海报。

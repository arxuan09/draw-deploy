---
name: seo-landing
description: 为 AI 绘画站点写「AI [主题] Generator」一类的工具落地页（/ai/<slug>），产出可直接粘贴进后台「内容运营 → 文章」的 Markdown：标题即搜索词、一句引言、本站真实示例、专属做法、FAQ。
---

来源：https://raw.githubusercontent.com/arxuan09/draw-deploy/main/skills/seo-landing/SKILL.md ，以公开仓最新版为准。

## 安装位置

- Claude Code：`.claude/skills/seo-landing/SKILL.md`
- Codex：`.codex/skills/seo-landing/SKILL.md`，或追加进 `AGENTS.md`
- Cursor：`.cursor/rules/seo-landing.md`

写之前先装好同目录的 `seo-article` 技能，并填好它第二节的「站点事实」：本技能只许写那里有的模型、价格和功能。

## 一、落地页和文章有什么不同

- 一页只对一个搜索意图（「AI anime art generator」「image to prompt」）。同义写法在引言里带一次，**不另开页**。
- 页面的主体是**示例**：6–8 张本站真实生成的图（视频页 3–4 段），每张带完整提示词和模型。示例由运营在站内出图后挑选，你只负责写说明，**不要编造图片地址或生成记录 ID**——没有的就留空，让运营从后台「从生成记录选」补上。
- 正文是**这一页专属**的做法：选哪个模型、为什么、提示词要点、做不到什么（至少写一种做不到的情形和替代做法）。只换主题词就能套到别页的段落，一页最多一段。
- 不写评分、用户数这类无法证实的数字。「free」只在注册赠送积分的范围内说，不写「unlimited free」。
- 不做名人、真人换脸、在世艺术家风格、IP 角色、去水印、NSFW 这类主题。

## 二、收录门槛（达不到页面照常可看，但 noindex、不进 sitemap）

- 示例：图片至少 6 张，或视频至少 3 段；每张都来自本站管理员账号的生成记录，全站不重复。
- FAQ 至少 3 条。
- 按钮带入的模型对用户可见。
- 译文行要人工改过标题和引言（机翻行不收录）。

## 三、输出格式（整篇贴进后台「粘贴导入」）

只输出下面这一个 Markdown 文件，不要解释。

```markdown
---
kind: ai
slug: anime-art-generator  # 英文 kebab-case，去掉开头的 ai
locale: en
title: AI Anime Art Generator
seo_title: "AI Anime Art Generator: Prompts, Models and Examples"
seo_description: Turn a one-line idea into anime art. See real examples with their full prompts, then try one with your sign-up credits.
excerpt: Turn a one-line idea into anime art, from character sheets to cinematic stills. Free to try with your sign-up credits.
published_at: 2026-11-04
tags:
  - styles  # 第一个标签是 /ai 总页的分组：models / tools / ecommerce / design / styles
  - anime
related:
  - fantasy-art-generator
launch:
  mode: image  # image / video / agent / ecommerce-detail / ecommerce-tryon
examples:
  - generation_id: 0b6f4c3e-0000-0000-0000-000000000000
    image: https://example.com/generated/anime-1.png
    caption: A rainy-night street in cel-shaded style
faq:
  - q: Is the anime art generator free?
    a: New accounts get trial credits, enough for several images. After that each image costs the credits shown on the button.
  - q: Which model should I pick for anime?
    a: Start with the model the button preselects; switch when you need readable text in the picture.
  - q: Can I keep the same character across images?
    a: Upload one picture of the character as a reference and describe only what changes.
---

## Write the prompt in this order

Subject first, then the setting, the light and the camera…

## What it cannot do yet

…
```

说明：
- `examples` 每项只写 `generation_id`、`image`（视频写 `video`）、`caption`。提示词、模型、比例由站点按生成记录自动填，写了也会被覆盖。
- 示例里的 `generation_id` 和 `image` 是占位，正式稿里留给运营从后台替换；没有就整段 `examples` 先不写。
- `launch.model_id` / `launch.prompt` 不写时，按钮用第一条示例的模型和提示词。
- 提示词保持英文，其他语言的页面也不翻译提示词。

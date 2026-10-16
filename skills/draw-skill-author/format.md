# 本站运营技能包格式 · draw-platform-skill/1

Draw Studio 后台「Agent 技能 → 运营 → 新建 → 上传技能包」只接受这个格式。后台**只校验、不改写**：任何一条不合格，整包被拒，并一次列出全部原因。

## 1. 目录结构

```
<slug>/
  SKILL.md          frontmatter + 正文(必需)
  examples/         示例图(可选,≤ 6 张,按文件名排序,第一张是封面)
  references/       参考图(可选,≤ 4 张,用途写在 frontmatter 的 references)
```

打包成 zip 时，`<slug>/` 目录本身放在 zip 根部（也可以把 `SKILL.md` 直接放在 zip 根部）。

- 图片只认 `.png` / `.jpg` / `.jpeg` / `.webp`，单张 ≤ 5 MB；后台会转成 WebP 存进站点配置的对象存储。
- 其他文件（脚本、文档、`.DS_Store` 等）一律忽略，上传结果里会列出被忽略的文件名。
- 压缩包 ≤ 20 MB，解压后 ≤ 40 MB，文件 ≤ 80 个；不能含 `..` 路径、链接文件、嵌套的 zip。
- 包里有图而站点没配对象存储时，整包被拒。

## 2. SKILL.md

```markdown
---
format: draw-platform-skill/1
slug: neon-food-poster
name: { zh: "霓虹美食海报", en: "Neon Food Poster" }
summary: { zh: "……做什么 + 什么时候用……", en: "……what it makes + when to use it……" }
placeholder: { zh: "……", en: "……" }
category: design
keywords: [poster, food, neon]
references:
  - { file: layout.webp, note: "借三段式版式与留白,不借菜品" }
---

# 霓虹美食海报

正文 Markdown……
```

### 字段

| 字段 | 必填 | 规则 |
| --- | --- | --- |
| `format` | 是 | 固定写 `draw-platform-skill/1`。缺了会被当成「通用技能包」拒收 |
| `slug` | 是 | 小写字母、数字、连字符，如 `neon-food-poster`，≤ 40 个字符；站内唯一，上传同名 slug 时后台会问是否替换 |
| `name.zh` / `name.en` | 是 | ≤ 30 字，单行 |
| `summary.zh` / `summary.en` | 是 | ≤ 200 字，单行；写「做什么 + 什么时候用」 |
| `placeholder.zh` / `placeholder.en` | 否 | ≤ 120 字，单行；用户选中技能时输入框里的示例说法，可带 `[占位符]` |
| `category` | 是 | 只能是下面分类表里的一个 key |
| `keywords` | 否 | ≤ 5 个，每个 ≤ 20 字，不含逗号 |
| `logo` | 不要写 | 站点不再显示技能 Logo（卡片用第一张示例图，没有示例图时用技能名排成的文字封面）；旧包里写了也只会被忽略 |
| `references` | 有参考图时 | 每项 `{ file, note }`：`file` 是 `references/` 里的文件名，`note` ≤ 60 字，说明借这张图的什么 |

### 写法限制（校验器只认这个子集）

- 每个字段一行 `key: value`；中英文对用一行 `{ zh: "…", en: "…" }`，值一律用双引号。
- `keywords` 用一行 `[a, b, c]`。
- `references` 每项一行 `  - { file: 名称, note: "…" }`。
- 双引号里的值不要换行；需要写双引号时用 `\"`。

### 正文

- Markdown，≤ 20000 字，不能再以 `---` 开头。
- 写给站内 Agent 看：它会在用户需要时加载这段正文，照着做。

## 3. 分类

<!-- 下表与站点代码的分类注册表一一对应，门禁测试会核对 -->

- `social` 社交媒体
- `ecommerce` 电商带货
- `design` 平面设计
- `academic` 科研学术
- `writing` 文案写作
- `other` 其他

## 4. 上传之后

- 新上传的技能是**草稿**：只有管理员在工作台的技能选择器里能看到、能试用。
- 在后台点「发布」后才对全部用户可见；可以随时撤回为草稿或临时停用。
- 正文和参考图对普通用户不可见，用户只看得到名称、简介、示例图。

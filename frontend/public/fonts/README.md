# 界面字体：更纱黑体 UI

AppShelf 的界面字体，随前端打包进程序，任何电脑上显示一致。`src/index.css` 的 `@font-face` 引用这里的文件。

| 文件                        | 字重                      | CSS 字重范围                    |
| --------------------------- | ------------------------- | ------------------------------- |
| `SarasaUiSC-Regular.woff2`  | 常规 400                  | 100–449                         |
| `SarasaUiSC-SemiBold.woff2` | 半粗 600                  | 450–649（`font-medium` 也用它） |
| `SarasaUiSC-Bold.woff2`     | 粗体 700                  | 650–900                         |
| `LICENSE-Sarasa-Gothic.txt` | 许可证原文（SIL OFL 1.1） | —                               |

## 来源

- [be5invis/Sarasa-Gothic](https://github.com/be5invis/Sarasa-Gothic) `v1.0.43`（tag commit `61e74fd`，2026-10-25），发布包 `SarasaUiSC-TTF-Unhinted-1.0.43.7z`。
- 更纱黑体 UI 简体：拉丁字母来自 Inter，汉字来自思源黑体。
- 许可证 SIL Open Font License 1.1，可以随软件分发和修改（含裁剪子集）；保留名称只有 Adobe 部分的「Source」，子集沿用原名「Sarasa UI SC」不受限制。分发时必须附带 `LICENSE-Sarasa-Gothic.txt`。

## 子集

完整字体每个字重约 13 MB，这里只保留界面会用到的字，每个字重约 1 MB：

- ASCII、拉丁补充与扩展 A、常用标点、字母符号、箭头、数学运算符、带圈数字、几何图形、中文标点、全角字符；
- GB2312 的全部字符（一二级汉字 6763 个及其符号）；
- `frontend/src` 下 `.ts`、`.tsx`、`.css` 里出现的每个字符。

保留全部 OpenType 特性（等宽数字 `tnum`、字距等），去掉 hinting，输出 WOFF2。子集外的字由字体栈里后面的 Noto Sans SC、微软雅黑 UI 补上。

界面新增了 GB2312 以外的字时，重新生成：用 fontTools 的 `subset` 按上面的范围裁剪三个字重（`flavor="woff2"`、`layout_features=["*"]`、`name_IDs=["*"]`、`hinting=False`），覆盖这里的文件。

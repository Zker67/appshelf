# secret

加密保存用户的机密信息，目前只用于设置里填写的 GitHub token。

| 函数 | 作用 |
|---|---|
| `Save(path, value)` | 用 Windows DPAPI 以当前用户的密钥加密后写入 `path`（先写临时文件再替换） |
| `Load(path)` | 读取并解密；文件不存在返回空字符串，不报错 |
| `Delete(path)` | 删除文件；不存在不报错 |

- 加密附带 AppShelf 自己的熵值，同一用户的其他程序直接调用 DPAPI 也解不开；文件复制到别的电脑或别的 Windows 用户下无法解密。
- 非 Windows 构建返回 `ErrUnsupported`。
- 错误信息只说明哪一步失败，不包含被保存的内容。

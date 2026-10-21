# c1mail — C1-Slim 纯文本邮件客户端

快易典 C1-Slim / MP-D261 墨水屏设备上的邮件收发（IMAP/SMTP）+ 本地缓存。
目标范围：**纯文本收发 + 本地缓存**（不做富文本、附件、日历、推送）。

## 组成

| 路径 | 说明 |
|---|---|
| `mail/` | 协议包：账号配置、极简 IMAP、SMTP 发信、RFC822 纯文本解析、本地缓存/发件箱 |
| `cmd/c1mail` | 设备应用（收件箱/阅读/写信/发件箱/设置） |
| `c1device` | 画布/输入（含完整 a-z 键盘映射） |

## 账号配置

在 PC 上编辑 `account.json`，推到设备：

```json
{
  "email": "you@example.com",
  "name": "你的名字",
  "password": "应用专用密码或授权码",
  "imapHost": "imap.example.com",
  "imapPort": 993,
  "smtpHost": "smtp.example.com",
  "smtpPort": 465,
  "ssl": true,
  "skipTls": false
}
```

| 字段 | 说明 |
|---|---|
| `ssl: true` | 隐式 TLS（IMAP 993 / SMTP 465） |
| `ssl: false` | STARTTLS（IMAP 143 / SMTP 587） |
| `skipTls` | 跳过证书校验，**仅测试** |

```bat
adb push account.json /storage/c1/local-apps/apps/mail/data/account.json
```

### 网易（163 / 126 / yeah）

| 项 | 值 |
|---|---|
| IMAP | `imap.163.com` 993（SSL） |
| SMTP | `smtp.163.com` 465（SSL） |
| 密码 | **授权码**（网页版设置 → POP3/SMTP/IMAP → 开启并生成） |

126 用 `imap.126.com` / `smtp.126.com`。模板：`account.163.example.json`。

### 微软（outlook / hotmail / Office365）

| 项 | 值 |
|---|---|
| IMAP | `outlook.office365.com` 993（SSL） |
| SMTP | `smtp.office365.com` 587（STARTTLS） |
| 密码 | 开了两步验证要用**应用密码**；企业版可能禁用基本认证（需 OAuth，本 MVP 未做） |

模板：`account.outlook.example.json`。端口决定 TLS 方式：993/465 隐式 TLS，587 走 STARTTLS。

Gmail / QQ 同理：应用专用密码或授权码，不要用登录密码。

## 按键

| 键 | 收件箱 | 阅读 | 写信 | 发件箱 |
|---|---|---|---|---|
| ↑↓ | 选择 | 滚动 | 切字段 | 选择 |
| OK / → | 打开 | — | 发送 | 重发 |
| c | 写信 | 回复 | — | — |
| r | 同步 | | | |
| o | 发件箱 | | | |
| s | 设置 | | | |
| 字母/数字 | — | — | 输入正文 | — |
| Back | 退出 | 回列表 | 回列表 | 回列表 |

## 跨应用剪贴板

约定共享文件：**`/storage/c1/clipboard.txt`**（UTF-8 纯文本，整文件覆盖）。

| 应用内按键 | 作用 |
|---|---|
| 阅读 **y** | 复制当前邮件（主题/发件人/正文）到共享剪贴板 |
| 写信 **p** | 从共享剪贴板粘贴到当前字段 |

其它应用只要读写同一路径即可互通：

```sh
# 查看
cat /storage/c1/clipboard.txt
# 写入（例如 shell）
printf '%s' "hello" > /storage/c1/clipboard.txt
```

PC 侧用 adb 同步：

```bash
adb pull /storage/c1/clipboard.txt clip.txt
adb push clip.txt /storage/c1/clipboard.txt
```

环境变量 `C1CLIPBOARD` 可覆盖路径（PC 测试）。

## 本地缓存

`/storage/c1/local-apps/apps/mail/data/`

- `account.json` — 账号
- `cache/inbox.json` — 收件箱索引
- `cache/messages/<uid>.json` — 已读正文
- `cache/outbox/` — 发送失败待重发

## 构建

```bat
set GOPATH=..\..\c1weread-work\gopath
set GOCACHE=..\..\c1weread-work\gocache
set GOPROXY=https://goproxy.cn,direct
set GOOS=linux& set GOARCH=mipsle& set GOMIPS=hardfloat& set CGO_ENABLED=0
go build -trimpath -ldflags "-s -w -buildid=" -o out\c1mail .\cmd\c1mail
```

**不修改 ELF e_flags**（保持 `0x50001004`）。

## 部署（local-apps）

```bat
adb shell mkdir -p /storage/c1/local-apps/apps/mail/bin
adb push out/c1mail /storage/c1/local-apps/apps/mail/bin/c1mail
adb shell chmod 0755 /storage/c1/local-apps/apps/mail/bin/c1mail
```

清单 `manifests/21-mail.json`（sha256 填实际值）：

```json
{
  "id": "mail",
  "name": "邮件",
  "executable": "/storage/c1/local-apps/apps/mail/bin/c1mail",
  "sha256": "..."
}
```

商店上架必须 `-mode direct`（与微信读书/墨页相同），否则 terminal 模式会 SIGHUP 闪退。

## 已知限制

- 只收发**纯文本**，HTML 邮件正文可能为空；不下载附件。
- IMAP 只操作 INBOX；无已发送同步（SMTP 发出即可）。
- 无推送，手动 `r` 同步；同步最近 50 封信头。
- 证书校验依赖系统 CA；设备 `/etc/ssl/certs` 为空时需打包 CA 或临时 `skipTls`。
- 撰写用全键盘；无拼音输入法。

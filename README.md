# TempMail

一个自托管临时邮箱平台，采用 **单二进制运行**：一个 Go 可执行文件同时提供 **前端页面 + HTTP API + SMTP 收件 + SQLite 存储**。

## 功能

- 临时邮箱创建与续期
- 多域名池
- 域名 MX 自动验证与定时复检
- API Key 鉴权
- 管理后台
- 留存邮件查看
- Cloudflare 域名辅助接口

## 默认端口

- HTTP: `8080`
- SMTP: `25`

常见映射：

- `80 -> 8080`
- `25 -> 25`

## 快速启动

```bash
go build -o tempmail .
cp .env.example .env
export $(grep -v '^#' .env | xargs)
./tempmail
```

启动后管理员 API Key 会写入：

```bash
cat data/admin.key
```

浏览器访问：

```text
http://<服务器IP>
```

## 关键环境变量

| 变量 | 说明 |
|------|------|
| `SMTP_SERVER_IP` | 服务器公网 IP，必填，用于 MX 校验和 SPF 提示 |
| `SMTP_HOSTNAME` | SMTP 对外主机名，例如 `mail.example.com` |
| `PORT` | HTTP 监听端口，默认 `8080` |
| `SMTP_PORT` | SMTP 监听端口，默认 `25` |
| `API_DB_PATH` | SQLite 路径，默认 `./data/tempmail.db` |
| `HTTP_ADDR` | 直接指定 HTTP 监听地址，优先级高于 `PORT` |
| `SMTP_ADDR` | 直接指定 SMTP 监听地址，优先级高于 `SMTP_PORT` |
| `SMTP_DOMAIN` | SMTP 对外标识域名，默认跟随 `SMTP_HOSTNAME`，否则为 `localhost` |
| `ADMIN_KEY_FILE` | 管理员 API Key 输出路径，默认 `./data/admin.key` |
| `RATE_LIMIT` | API 限流阈值，默认 `500` |
| `RATE_WINDOW` | API 限流窗口秒数，默认 `60` |
| `SMTP_MAX_MESSAGE_BYTES` | 单封邮件最大字节数，默认 `10240000` |
| `SMTP_MAX_RECIPIENTS` | 单次会话最多收件人数，默认 `100` |
| `SMTP_READ_TIMEOUT` | SMTP 读超时秒数，默认 `30` |
| `SMTP_WRITE_TIMEOUT` | SMTP 写超时秒数，默认 `30` |

## 添加域名

推荐 DNS 记录：

```text
MX   @   mail.yourdomain.com   优先级 10
TXT  @   v=spf1 ip4:<服务器IP> ~all
```

系统会：

- 每 30 秒轮询待验证域名
- 每 6 小时重检已激活域名
- 验证通过后直接开始 SMTP 收件

## 邮件接收流程

```text
发件方服务器
   │
   │ 1. 通过 DNS 查询 MX（这里会用到 53 端口）
   ▼
DNS / MX 记录
   │
   │ 2. 得到目标主机，例如 mail.yourdomain.com
   ▼
TempMail SMTP 服务 :25
   │
   │ 3. Go 程序接收 SMTP 邮件
   │ 4. 校验收件域名是否为激活域名
   │ 5. 解析原始邮件（From / Subject / Text / HTML）
   ▼
SQLite
   ├─ 邮箱存在      -> emails
   └─ 邮箱不存在    -> retained_mails
   
前端 / API
   │
   └─ 6. 从 SQLite 读取邮件并展示
```

一句话：**53 端口负责 DNS 查询，25 端口负责真正收邮件。**

## 认证方式

```http
Authorization: Bearer tm_xxxxxxxxxxxx
```

也支持：

```text
?api_key=tm_xxxxxxxxxxxx
```

## 测试

```bash
go test ./...
```

## 许可证

MIT

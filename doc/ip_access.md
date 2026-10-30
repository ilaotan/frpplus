# 客户端 IP 准入控制（IP Access）

> 本特性是 frp 的本地增强。默认只作用于 **tcp** 代理，可通过 `types` 打开
> `http` / `https` / `tcpmux`；visitor 型代理（stcp / xtcp / sudp）不支持。

## 1. 解决什么问题

tcp 代理的链路是这样的：

```
客户端 ──> frps:remotePort ──[work conn]──> frpc ──> 内网服务 127.0.0.1:localPort
```

内网服务看到的连接来源永远是 `127.0.0.1`（frpc 本机），真实客户端 IP 只在 **frps** 侧存在。
如果要做"只允许某些 IP 访问这个端口"，只能在 frps 上做判断。

frp 自带的 `proxy_protocol`（PROXY protocol）也能传递真实 IP，但它要求内网服务自己解析
PROXY 协议头——很多现成服务（数据库、老业务）不支持。本特性不改变隧道内数据，直接在服务端
按 IP 放行或拦截，对 frpc 和内网服务都是透明的。

## 2. 工作原理

拦截点在 `server/proxy/proxy.go` 的 `handleUserTCPConnection`：

```
客户端连接进来
   │
   ├─ 取 userConn.RemoteAddr()  → 真实客户端 IP
   ├─ IP Access 判定：模式 + 规则（+ 记录这次访问）
   │     └─ 拒绝 → 直接关闭连接，不分配 work conn
   ├─ server plugin 钩子（原有逻辑）
   └─ 分配 work conn → 转发
```

要点：

- 判定发生在**分配 work conn 之前**，被拦的连接不消耗 frpc 侧资源。
- 每个 TCP 连接都会写入一条访问记录（同一 代理+IP 只累加，不新增行）。
- 判定只针对 `tcp` 类型；udp / http / https / tcpmux / stcp 等不受影响。

## 3. 快速开始

```toml
# frps.toml
bindPort = 7000
auth.token = "your-token"

webServer.addr = "0.0.0.0"
webServer.port = 7500
webServer.user = "admin"
webServer.password = "your-password"

[ipAccessControl]
enable = true
storePath = "/var/lib/frp/ipaccess.json"   # 建议绝对路径
defaultMode = "allowlist"                  # 先收紧，再按需放行
retentionHours = 168
maxRecords = 5000
enableSelfAllow = false
selfAllowHours = 24
```

重启 frps 后打开 Dashboard → 左侧 **IP Access**。

> `defaultMode = "allowlist"` 且一条规则都没有时，**该端口的所有连接都会被拒**。
> 稳妥的做法是先设 `off`，在记录里观察一段时间真实来源，再切到黑白名单。

## 4. 模式与匹配规则

三种模式：

| 模式 | 行为 |
|---|---|
| `off` | 不拦截，只记录（默认） |
| `allowlist` | 只放行命中 allow 规则的 IP，其余全拒 |
| `denylist` | 命中 deny 规则的 IP 拒绝，其余放行 |

生效范围：**全局模式 + 按代理覆盖**。某个代理没单独设置时继承全局。

匹配规则：

1. 先看该代理自己的规则，没有命中再回落到全局规则；
2. 规则支持单个 IP 和 CIDR（`10.0.0.0/8`）；
3. 多条命中时**最长前缀优先**（越精确越优先）；
4. 前缀长度相同时 **deny 优先**（防止误放宽）；
5. 过期规则（自助加白产生）在匹配阶段直接忽略，不用等清理。

例子：全局 allow `10.0.0.0/8`，再给 `10.9.9.9` 加一条 deny，则 `10.1.2.3` 放行、`10.9.9.9` 被拒。

## 5. 配置项

| 配置 | 默认 | 说明 |
|---|---|---|
| `enable` | `false` | 总开关。关闭时不做判定，也不记录 |
| `storePath` | `./.frps_ipaccess.json` | 规则与模式的数据文件（不含访问记录），建议写绝对路径 |
| `defaultMode` | `off` | 初始全局模式；运行时可改，改动会持久化 |
| `retentionHours` | `168` | 内存中访问记录的保留时长（按最后访问时间） |
| `maxRecords` | `5000` | 内存中缓存的记录条数上限，超出后淘汰最久未访问的（不落盘） |
| `types` | `["tcp"]` | 生效的代理类型，可选 `tcp` / `http` / `https` / `tcpmux` |
| `enableSelfAllow` | `false` | 是否开放"自助加白"接口 |
| `selfAllowHours` | `24` | 自助加白的默认有效期，可被请求参数覆盖 |

## 6. 管理接口

### 6.1 通用约定

- **鉴权**：Dashboard 的 Basic Auth（`webServer.user` / `webServer.password`），没有额外的 token；
- **编码**：请求与响应都是 UTF-8 JSON，写操作必须带 `Content-Type: application/json`；
- **返回**：`{"code":200,"msg":"success","data":...}`；失败时 `code` 是 HTTP 状态码，`msg` 是原因；
- **地址**：示例用 `http://192.168.1.10:7500`，换成你 frps 的实际地址。

先设两个变量，后面所有命令直接复用：

```bash
BASE=http://192.168.1.10:7500/api/v2/ip-access
AUTH=admin:your-password          # = webServer.user:webServer.password
```

Windows 上两个必踩的坑：

```bat
:: 1) cmd 里没有 curl，Windows 10/11 自带 curl.exe，直接写 curl 即可
:: 2) 机器上设了 http_proxy 时，本地请求要加 --noproxy "*"，否则会被送去代理

curl.exe -s --noproxy "*" -u admin:your-password http://192.168.1.10:7500/api/v2/ip-access/settings
```

> 在 Windows 的终端里如果 `curl` 报"找不到参数"或返回 `Invoke-WebRequest` 的对象，说明用的是那个同名别名，写 `curl.exe` 就对了。

`-u` 只是把凭证编码成 `Authorization: Basic base64(user:pass)`，等价的手写请求头：

```bash
# base64('admin:your-password') = YWRtaW46eW91ci1wYXNzd29yZA==
curl -s -H "Authorization: Basic YWRtaW46eW91ci1wYXNzd29yZA==" \
     -H "Accept: application/json" \
     "$BASE/settings"
```

### 6.2 接口清单

| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/settings` | 全局模式、规则数、记录数、保留策略、生效的代理类型 |
| PUT | `/settings` | 切全局模式 |
| PUT | `/proxy-modes` | 单代理模式覆盖，`mode:""` 表示继承全局 |
| GET | `/rules` | 规则列表，支持 `proxyName` / `action` / `page` / `pageSize` |
| POST | `/rules` | 新增规则 |
| DELETE | `/rules/{id}` | 删除规则 |
| GET | `/records` | 访问记录，支持 `proxyName` / `ip` / `result=allowed\|denied` / 分页 |
| POST | `/records/delete` | 按 id 批量删除记录 |
| POST | `/prune` | 立即清理过期规则与记录 |
| GET/POST | `/self-allow` | 自助加白，见第 7 节 |
| GET | `/current-ip` | 回显请求方地址与是否回环 |
| POST | `/allow-current-ip` | 把当前请求方 IP 加进全局 allow，`hours:0` 为永久 |

### 6.3 curl 示例

**查看设置**

```bash
curl -s -u "$AUTH" "$BASE/settings"
```

```json
{"code":200,"msg":"success","data":{
  "enabled":true,"storePath":"/var/lib/frp/ipaccess.json","globalMode":"allowlist",
  "ruleCount":2,"recordCount":18,"retentionHours":168,"maxRecords":5000,
  "proxyModes":[],"types":["tcp"]}}
```

**切换全局模式**

```bash
curl -s -u "$AUTH" -X PUT -H "Content-Type: application/json" \
     -d '{"globalMode":"allowlist"}' "$BASE/settings"
```

**设置 / 取消单个代理的模式**（`mode` 传空串表示回到继承全局）

```bash
curl -s -u "$AUTH" -X PUT -H "Content-Type: application/json" \
     -d '{"proxyName":"web","mode":"denylist"}' "$BASE/proxy-modes"

curl -s -u "$AUTH" -X PUT -H "Content-Type: application/json" \
     -d '{"proxyName":"web","mode":""}' "$BASE/proxy-modes"
```

**新增规则**（`proxyName` 用 `*` 表示全局，`ip` 可以是单地址或 CIDR）

```bash
curl -s -u "$AUTH" -X POST -H "Content-Type: application/json" \
     -d '{"proxyName":"*","action":"allow","ip":"203.0.113.7","note":"office"}' \
     "$BASE/rules"
```

```json
{"code":200,"msg":"success","data":{"id":7,"proxyName":"*","action":"allow",
 "ip":"203.0.113.7","note":"office","createdAt":1793363639,"source":"manual"}}
```

**查规则 / 删规则**

```bash
curl -s -u "$AUTH" "$BASE/rules?page=1&pageSize=50"
curl -s -u "$AUTH" "$BASE/rules?action=deny"
curl -s -u "$AUTH" -X DELETE "$BASE/rules/7"
```

**查访问记录**（`result` 取 `allowed` / `denied`）

```bash
curl -s -u "$AUTH" "$BASE/records?page=1&pageSize=50"
curl -s -u "$AUTH" "$BASE/records?result=denied&ip=203.0.113.7"
```

**删记录 / 立即清理**

```bash
curl -s -u "$AUTH" -X POST -H "Content-Type: application/json" \
     -d '{"ids":[3,4]}' "$BASE/records/delete"

curl -s -u "$AUTH" -X POST "$BASE/prune"
```

```json
{"code":200,"msg":"success","data":{"cleared":2,"total":16}}
```

**当前请求方 IP**（只读，不产生规则）

```bash
curl -s -u "$AUTH" "$BASE/current-ip"
```

```json
{"code":200,"msg":"success","data":{"ip":"203.0.113.7","loopback":false}}
```

**一键放行当前 IP**（`hours:0` 表示永久；允许回环地址）

```bash
curl -s -u "$AUTH" -X POST -H "Content-Type: application/json" \
     -d '{"hours":24}' "$BASE/allow-current-ip"

curl -s -u "$AUTH" -X POST -H "Content-Type: application/json" \
     -d '{"hours":0}' "$BASE/allow-current-ip"
```

## 7. 自助加白（IP 会变的场景）

手机流量、动态宽带这类公网 IP 经常变的场景，每次改了 IP 都去页面加一条太麻烦。
开启后可以"敲门"式加白——**接口只把调用方自己的地址加进全局 allow，不接受传入 IP**。

前提：配置里 `enableSelfAllow = true`，且 `webServer.user` / `webServer.password` 都不为空。

### 7.1 GET：手机书签式

```bash
curl -s -u "$AUTH" "$BASE/self-allow?hours=12"
```

不传 `hours` 就用配置的 `selfAllowHours`（默认 24）。存成手机浏览器书签时，把凭证放进 URL
（浏览器会提示一次并记住），打开即完成加白，无需输入任何命令。

### 7.2 POST：脚本定时调用

`hours` 放 body 或 query 都可以，**query 优先**：

```bash
# body
curl -s -u "$AUTH" -X POST -H "Content-Type: application/json" \
     -d '{"hours":12}' "$BASE/self-allow"

# query
curl -s -u "$AUTH" -X POST "$BASE/self-allow?hours=12"

# 都不带 → 用配置里的 selfAllowHours
curl -s -u "$AUTH" -X POST "$BASE/self-allow"
```

手写请求头（放进脚本或其它语言里更直观）：

```bash
curl -s -X POST \
     -H "Authorization: Basic YWRtaW46eW91ci1wYXNzd29yZA==" \
     -H "Content-Type: application/json" \
     -H "Accept: application/json" \
     -d '{"hours":12}' \
     "http://192.168.1.10:7500/api/v2/ip-access/self-allow"
```

### 7.3 返回与含义

```json
{"code":200,"msg":"success","data":{
  "ip":"203.0.113.7","proxyName":"*","action":"allow",
  "ruleId":12,"expiresAt":1793407239}}
```

- `ip`：**服务端看到的来源地址**，不是你传的（你也没法传）；
- `ruleId`：落在哪条规则上，重复调用不变；
- `expiresAt`：unix 秒；`0` 表示永久（只有 `allow-current-ip` 能产生永久规则）。

连续调用只续期、不新增行：

```bash
curl -s -u "$AUTH" "$BASE/self-allow?hours=1"     # ruleId 12, expiresAt = now+1h
curl -s -u "$AUTH" "$BASE/self-allow?hours=24"    # 仍 ruleId 12, expiresAt = now+24h
curl -s -u "$AUTH" "$BASE/self-allow?hours=1"     # 已有有效期更长，原样返回，不变
```

### 7.4 行为约定

- **只加全局 allow 规则**，不支持指定代理；
- **IP 不由调用方提供**，一律取 TCP 连接的来源地址；
- **幂等**：同一 IP 反复调用只保留一条规则，`ruleId` 不变；已有有效期更长时不会被缩短，
  间隔足够时按新的 `hours` 顺延（适合定时器）；
- 有效期默认取 `selfAllowHours`，可用 `hours` 覆盖，上限 **720 小时（30 天）**；
- 到期后规则自动失效并被清理。

定时调用建议：**间隔 ≤ hours / 2**，例如 `hours=12` 就每 5~10 分钟调一次。

### 7.5 常见错误

| HTTP | msg 关键字 | 原因与处理 |
|---|---|---|
| 401 | `Unauthorized` | 账号密码错，或忘了 `-u` |
| 403 | `self allow requires webServer.user and webServer.password to be set` | Dashboard 没设密码。frp 的 Basic Auth 在账号密码都为空时是**全放行**的，所以这里显式拒绝 |
| 400 | `self allow is not enabled` | 配置里 `enableSelfAllow = false`，改完重启 frps |
| 400 | `the request comes from 127.0.0.1` | 请求经过了本机反向代理，取到回环地址；直连 frps 再调 |
| 400 | `hours must be a positive integer, 720 is the maximum` | `hours` 传了 0、负数或非数字（要永久请用 `allow-current-ip`） |
| 404 | `page not found` | 二进制太旧，不含这个接口，用新版本重编 |

安全设计（都是故意的）：

| 措施 | 原因 |
|---|---|
| 必须配置了 `webServer.user` / `webServer.password`，否则返回 403 | frp 的 Basic Auth 中间件在账号密码都为空时是**全放行**的 |
| 来源是回环地址时返回 400 | 说明请求经过了本机反向代理，加 `127.0.0.1` 没有意义，直接报错而不是静默加一条无效规则 |
| 开关默认关闭 | 升级不会凭空多出一个入口 |
| `hours` 上限 720 | 避免一次调用把有效期设到天荒地老 |

## 8. Dashboard

`http://<frps>:7500` → **IP Access**：

- **Mode**：切换全局模式，查看/设置单个代理的覆盖；
- **Rules**：增删规则，`Expires` 列显示有效期（`permanent` 表示永久），`api` 标记表示自助加白产生；
  右上角 **Allow my IP** 按钮可一键把"页面上看到的本机 IP"加进白名单，下拉里选有效期：
  `1 hour` / `24 hours` / `7 days` / `Permanently`；
- **Access records**：按代理/IP/结果筛选，可对某一行直接 **Allow / Deny** 生成规则，支持批量删除与立即 Prune。

`Allow my IP` 是**管理动作**（走 Dashboard 账号），与自助加白接口的区别：

- 不依赖 `enableSelfAllow`；
- 允许回环地址——你在 frps 本机打开页面时，加的就是 `127.0.0.1`，这是合理需求；
- 页面头部会显示 `Your IP: x.x.x.x`，本机访问时标 `(local)`，点击前就能确认加的是哪个地址。

页面不自动刷新，点右上角 **Refresh** 获取最新记录。

## 9. 数据存储与清理

- 单文件存储（`storePath`）**只保存规则、按代理覆盖和全局模式**，写入走"临时文件 + rename"，
  进程崩溃不会留下半个文件；典型体积是几 KB；
- **访问记录不落盘**，只存在于内存（默认上限 5000 条），重启即清空；
- 所有规则变更（增删规则、切模式、自助加白、页面一键放行）都是**同步写盘**，进程被 kill 也不会丢；
- 每 5 分钟清理一次过期记录与过期规则，也可在页面或调用 `prune` 手动触发；
- 内存记录三重淘汰：按 `retentionHours` 过期 → 超过 `maxRecords` 时淘汰最久未访问的 → 突发大量新 IP 时即时裁剪；
- 规则和模式修改后重启 frps 依然有效。

## 10. 本地验证

仓库 `bin/` 下已带好可直接跑的配置（`bin/frps.toml` / `bin/frpc.toml`），
在 `bin` 目录开三个终端：

```powershell
python -m http.server 18080     # 被代理的目标服务
.\frps.exe -c frps.toml         # dashboard: http://127.0.0.1:17500  admin/admin123
.\frpc.exe -c frpc.toml         # 把 127.0.0.1:18080 暴露到 18000
```

验证顺序（`allowlist` + 空规则开局）：

1. 访问代理端口 → 被拒；
2. 打开 Dashboard 的 IP Access → 记录里出现该 IP，`allowed=false`；
3. 加一条 allow 规则（或在页面点该行的 **Allow**）→ 再访问 → 正常；
4. 切到 `denylist`，把该 IP 加进 deny → 再访问 → 又被拒。

验证自助加白时注意：从本机 `127.0.0.1` 调用会被回环校验拒掉（400），
要用局域网 IP 访问一次，例如 `http://192.168.1.10:17500/api/v2/ip-access/self-allow`。

## 11. 已知限制

- 生效范围由 `types` 决定：`tcp`（默认）、`http`、`https`、`tcpmux`。
  `udp` / `sudp` 未实现（每包判定需要额外的节流设计）；
  `stcp` / `xtcp` 不支持——frps 看到的"用户连接"其实是 visitor（另一个 frpc），
  拦到的是 visitor 的地址，管不到最终用户。
- `http` 场景下，若 frps 前面还有 nginx / CDN，`RemoteAddr` 是代理的地址；此时规则应针对代理 IP，
  或者改用 `https` + 在代理上做真实 IP 透传。
- 移动网络的出口是 CGNAT 共享地址，把"自己的 IP"加白，实际放行的是该 NAT 出口背后的一批用户，
  frps 层面无法区分。这是场景固有问题。
- 如果 frps 前面还有**公网**反向代理或 CDN，`RemoteAddr` 会是代理的地址，此时加白的是代理 IP
  （回环反代的情况已被显式拒绝）。
- 同一 IP 若已存在手工 deny 规则，同前缀长度下 deny 优先，此时自助加白会返回成功但实际仍被拦。

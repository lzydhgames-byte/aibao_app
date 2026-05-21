# 安全与合规

## 已接触的（Plan 1 起）

### 密钥永远不能进 Git
**一句话**：Git 历史是永久的，一旦 commit 密钥即使下个 commit 删掉，仓库快照里依然存在，任何能拿到仓库的人都能翻出来。

详见 [01.7 .gitignore](01-git.md#17-gitignore)。

### 配置外置（12-Factor App / Config）
**一句话**：代码里**绝不**写死配置。配置通过环境变量注入。

详见 [05.4 12-Factor App](05-software-design.md#54-12-factor-app十二要素)。

### 端口仅监听 127.0.0.1
**一句话**：本地依赖（PG / Redis）只对本机开放，不暴露给局域网/公网。

详见 [04.5 端口映射安全细节](04-docker.md#45-端口映射-127001543254320安全细节)。

## 10.4 哈希（hash）
把任意长度输入"压缩"成固定长度乱码。**类比**：搅拌机——苹果变果泥，但还原不回。  
特性：① 同输入恒同输出 ② 不可逆 ③ 改一字哈希面目全非。  
SHA256 是主流算法，输出 64 个十六进制字符。  
**为什么需要**：隐私关联场景（同一个用户多条日志关联）只用 hash 就够，不需要原文，且即便日志泄露也还原不回。

## 10.5 加盐（salt）
直接哈希有"彩虹表攻击"——攻击者预制好"所有手机号→哈希"的对照表反查。  
**解决**：哈希前加一段只有你知道的密钥：`sha256(salt + ":" + value)`。  
**类比**：苹果先抹自家秘制酱料再搅。别人有搅拌机也复现不出。  
**为什么需要**：手机号空间小（11 位约 100 亿），无盐 hash 一夜就能枚举完。盐让攻击者必须先偷到盐才能枚举。  
项目体现：safehash 包 `Hasher{salt}.HashString(value)` → `h_<12 字符>`。

## 10.6 三种脱敏方式（hash / mask / redact）
| 方式 | 输出 | 何时用 |
|---|---|---|
| **hash** | `h_a3f8c21d4e2b` | 需要"同一对象多条日志关联"但绝不可还原（如 child_id） |
| **mask 打码** | `138****8000` | 客服需要模糊辨认大致对象（如手机号） |
| **redact 抹除** | `len=23` | 完全不需要内容，只统计存在/长度（如 prompt 文本） |

铁律：日志中**绝不**出现明文手机号、孩子姓名、prompt 完整内容、API Key。

## 10.7 JWT（JSON Web Token）
"自包含"的字符串令牌，里面装着用户身份和过期时间，并由服务端签名防伪。  
形如 `xxxxx.yyyyy.zzzzz`——三段：Header（算法）/ Payload（用户 id、过期时间等）/ Signature（防篡改）。  
**类比**：景区门票上印着姓名、入园时间、防伪水印。检票员只看水印就知是真票，不必每次回总部查。  
**为什么需要**：传统"server 存 session"在多机部署/扩容时要么粘连同一台机要么共享存储。JWT 让任何一台 server 都能独立验证——**无状态**，水平扩展几乎零成本。  
项目体现：`jwtauth.Manager` 签发 + 解析；客户端登录后保存 access_token，每次请求带 `Authorization: Bearer <token>`。

## 10.8 HMAC vs 非对称签名（HS256 / RS256）
JWT 签名算法的两大家族：
- **HS256**（HMAC-SHA256，对称）：签和验**用同一个密钥**。简单、快、密钥短；缺点是验证方也得知道密钥
- **RS256**（RSA-SHA256，非对称）：用**私钥签、公钥验**。多服务场景理想（公钥可分发，私钥锁紧）；签验慢一点，密钥更长

**类比**：HS256 = 同一把钥匙开关锁；RS256 = 一把钥匙锁、另一把钥匙开（公私钥）。  
**为什么 MVP 用 HS256**：单后端服务自签自验，没有"分发公钥让别人验"的场景。简单够用。等以后有"前端 / 第三方业务想验我们的 token"时再升 RS256。  
项目体现：`jwt.SigningMethodHS256` + 配置里的 `auth.jwt_secret`。

## 10.9 Access Token vs Refresh Token
双 token 模式：
- **Access Token**：短命（24h），每次请求都带，用于鉴权
- **Refresh Token**：长命（7d），仅在 access 过期时拿来换新的 access

**为什么需要双 token**：access 暴露面大（每次请求都发），如果泄露危害有限（最多 24h 被盗用）；refresh 暴露面小（仅在续期时用），即使长命也较安全。这是"小损失止损"的安全权衡。  
项目体现：`Manager.IssueAccess` / `Manager.IssueRefresh`，且 `ParseAccess` 拒绝把 refresh 当 access 用（`Type` 字段防混用）。  
注意：refresh 的"续期接口"本 plan 暂未做（MVP 用户重新登录也行），**待后续 plan 补**。

## 10.10 AES-256-GCM 对称加密 + Nonce
对称加密 = 加和解用同一把钥匙。AES-256-GCM 是工业标准：
- **AES-256**：256 位密钥（32 字节）的高级加密算法
- **GCM**（Galois/Counter Mode）：除了加密还自带"防篡改校验"（哪怕一字节被改也会拒绝解密）
- **Nonce**（一次性数字）：每次加密用一个全新的随机值。**同样的明文每次加密结果都不同**——防止攻击者通过观察"密文重复"推测信息

**类比**：保险箱（AES-256）+ 每次开锁都换密码盘起始位置（Nonce）+ 内置"撬动报警"（GCM 校验）。  
**为什么需要 nonce**：如果固定 nonce，加密 100 个相同手机号会得 100 个相同密文——攻击者一眼看出哪些用户手机号一样。Nonce 保证密文随机化。  
项目体现：`phonecrypt.Cipher` 用 AES-256-GCM；每次 Encrypt 生成 12 字节随机 nonce 拼在密文前面，Decrypt 时取出 nonce 还原。

## 10.11 云服务标识符的"完整名 vs 短名"陷阱

很多云服务（Tencent COS / AWS S3 + 账户后缀 / 阿里 OSS）的 bucket / 资源在控制台显示成"完整名"（含账号 ID 后缀），但 SDK 期望传入"短名" + 单独的 AppID / AccountID 字段，**SDK 内部自动拼接**。运维者很容易把完整名当 bucket 名传入 → SDK 又拼一次 → 得到 `<short>-<appid>-<appid>` 一个根本不存在的资源 → 404 NoSuchBucket。

**项目体现**（Plan 5）：腾讯云 COS 控制台显示 `aibao-audio-1234567890`，用户把这个完整名贴进 `COS_BUCKET` 配置。但 cos-go-sdk-v5 期望传入短名 `aibao-audio` + AppID `1234567890`——SDK 内部拼成 `aibao-audio-1234567890`。结果配置传入完整名后，代码访问的是 `aibao-audio-1234567890-1234567890`，404。

**生产代码必须双向容错**——检测传入是否已含后缀：
```go
if !strings.HasSuffix(cfg.Bucket, "-"+cfg.AppID) {
    cfg.Bucket = cfg.Bucket + "-" + cfg.AppID
}
```
不管运维填"短名"还是"完整名"，代码都能识别。见 `internal/gateway/storage/cos.go`（commit 2f8f0f3）。

**为什么需要这条**：这是云厂商 API 设计的常见缺陷——同一个标识符有两种合法形态，**文档+控制台用一种、SDK 用另一种**。运维者按"我从控制台拷贝的就是 bucket 名"的直觉操作必踩。代码不做容错就只能等线上炸。

**类比**：手机号"+86 区号"陷阱——有人输 `13900000000` 有人输 `+8613900000000`，系统得能识别两种形态再统一。或者：邮编"五位 vs 九位"——美国 zip 可以 `94103` 也可以 `94103-1234`，软件得双向兼容。

**同样的陷阱出现在**：
- AWS S3：`<bucket>` vs `<bucket>.s3.amazonaws.com` vs `arn:aws:s3:::<bucket>`
- 阿里 OSS：`bucket` + `endpoint` 分离，控制台显示却是 `bucket.endpoint` 合一
- 任何"resource ID + tenant ID"分离的云服务

排查口诀：**API 返回 NotFound 时，第一个怀疑的不是权限，是你传的 ID 形态对不对**。

## 10.12 红线词单字陷阱：词表的"误报代价 vs 漏过代价"

🎓 内容过滤如果用**单字子串匹配**，会大量误伤无害词，反而让正常内容走 fallback / 降级路径。

**爱宝项目活案例**：`rules.yaml` 里把单字「刺」和「砍」当红线词。

| 红线单字 | 真正想拦 | 实际被误伤 |
|---|---|---|
| 刺 | 刺杀 / 刺伤 | 刺猬、玫瑰花的刺、刺啦一声、刺绣、热刺队 |
| 砍 | 砍杀 / 砍人 | 砍柴、砍价、砍倒大树、砍头价 |
| 杀 | 杀害 | 自杀（仍要拦）、杀虫剂、杀青、杀手锏 |

**今天踩的雷**：8 分钟童话故事 LLM 长度重写到 1879 字（接近目标），最后 PostCheck 命中「刺」单字（实际是"刺猬"或"刺啦"无害用法）→ fallback → 1 分钟音频。

**修法原则**：
1. **红线词必须用最小完整意图组合**：「刺死」「刺伤」「拿刀刺」，而不是单字「刺」
2. **保留语义独立的单字**（极少数）：「杀」可能要拦但要看上下文——更稳的方式是组合：「杀死」「杀害」「自杀」
3. **每个红线词上线前问"它会不会误伤童话里高频出现的无害词"**

**判断口诀**：
- 拿这个单字搜 5 个常见无害童话词
- 至少 4 个误伤 → 直接删
- 1-2 个误伤 → 改成组合词
- 0 误伤 → 留下

**为什么需要**：内容过滤的误报代价（用户拿到 80 字罐头）经常**远超**漏过代价（一个本来正常的故事被放行）。**过严的红线词不是"更安全"，是"更糟的 UX"**。

**何时引入**：Plan 9c / commit `5549e87`。

## 10.13 一次性消费 vs 重试容忍：UX 决定语义

🎓 同一个"用过即销毁"的设计在不同业务里效果完全相反。

**爱宝 SMS 验证码踩的雷**：
```go
// 原实现：Redis GETDEL 原子操作
stored := codeStore.Take(ctx, phoneHash)  // 读 + 删
if stored != userInput {
    return errMismatch
}
// 用户输错 1 次 → 验证码已被删 → 输对的也提示"已过期"
```

**业务真相**：
- **支付 / 兑换券**：必须一次性，用过就废，错的也算用过
- **登录验证码**：应该允许 TTL 窗口内多次重试，**只有匹配成功才算消费**

**修法**：拆原子操作为 `Peek`（GET 不删）+ `Consume`（DEL，仅在匹配后调）
```go
stored := codeStore.Peek(ctx, phoneHash)   // 只读
if stored != userInput {
    return errMismatch                      // 错的不消费
}
codeStore.Consume(ctx, phoneHash)           // 对的才删
```

**判断口诀**：用过即销毁前问一句"输错了重试是不是合理诉求？"
- 是 → 拆 Peek + Consume
- 否（金融 / 抽奖 / 限量） → 保留原子销毁

**为什么需要**：技术选型不能脱离业务语义。Redis GETDEL 是个**强工具**，但用在登录场景下变成了反 UX 设计。

**何时引入**：Plan 9b smoke / commit `2488d85`。

## 10.14 PreCheck / PostCheck 对称设计原则

🎓 安全管道经常分前置（用户输入端）和后置（LLM 输出端）。**两边应该用同一套分级宽容规则**——否则其中一边过严就成系统短板。

**爱宝项目活案例**（Plan 9c 第二战发现）：
- PostCheck 把 horror + negative_values 改 warn-only 后，效果显著（fallback 率从 30% → 7%）
- 但**PreCheck 没改**——同一个 rules.yaml 的全部红线词在 PreCheck 仍 hard-fail
- 结果：用户输入"不要嘲笑别人"（合理的反义教育 prompt）→ PreCheck 命中「嘲笑别人」（negative_values 类）→ 直接 400 拒绝

**症结**：内容过滤上下游的"放行尺度"不一致——上游严卡，下游温和，等于上游决定了下限。

**设计原则**：
1. **共享规则源**：Pre/PostCheck 都从 `rules.yaml` 同一份数据源加载，不要双份维护
2. **共享分类信息**：`PreCheckOutput` 和 `PostCheckOutput` 都要返回 `MatchedCategory`
3. **共享分级表**：哪些 category 是 hard-fail / warn-only 在一个地方定义，两端引用
4. **可以单边收紧**：例如 dangerous_imitation 可以 PreCheck 严卡（防恶意），PostCheck 也严卡——但不要 PreCheck 严+PostCheck 松或反过来

**为什么需要**：安全管道是个"水桶"，最短的那块板决定整体性能。如果 PostCheck 设计精巧但 PreCheck 一刀切，PostCheck 改了也白改——用户根本走不到 PostCheck。

**何时引入**：Plan 9c 第二战收尾发现 / 第三战待修。

## 10.14 云存储 4xx 错误码三分法 + SecretKey 一次性显示

**一句话**：对象存储（COS / S3 / OSS）拒绝你时，`InvalidAccessKeyId`、`SignatureDoesNotMatch`、`AccessDenied` 是三个完全不同的错误，指向三种不同的修法——看错误码比看 Message 文字更可靠。

**三分法对照**：

| 错误码 | 含义 | 修法 |
|---|---|---|
| `InvalidAccessKeyId` | 这把密钥根本不存在 / 拼错了 | 检查 SecretId 是否填错、是否被删 |
| `SignatureDoesNotMatch` | 密钥存在，但签名算错了 | 检查签名算法、SecretKey、时钟、待签字符串拼装 |
| `AccessDenied` | 密钥对、签名也对，但**这把密钥没有这个资源的权限** | 换有权限的密钥，或给资源加授权策略 |

**生活类比**：进小区门禁——
- `InvalidAccessKeyId` = 你刷的卡是张废卡/假卡
- `SignatureDoesNotMatch` = 卡是真的但你刷反了/消磁了
- `AccessDenied` = 卡是真的、也刷对了，但这张卡没开通这栋楼的权限

**为什么需要**：三个错误长得像（都是"被拒"），但乱猜会浪费大量时间。爱宝 Plan 10 Day 2 实战：换新 COS 桶后报 403 `AccessDenied`——因为是 `AccessDenied` 而不是 `InvalidAccessKeyId`，立刻能断定"密钥本身有效，只是没这个新桶的权限"，直接去查密钥归属，而不是浪费时间核对 SecretId 有没有打错一位。

**配套陷阱：SecretKey 只显示一次**。腾讯云、AWS、阿里云的 SecretKey 都只在「新建密钥」那一刻显示一次，关掉弹窗后永久不可查看（安全设计，防反复偷看）。
- **正确做法**：新建时立刻「下载 CSV」或直接写进配置文件
- **丢了怎么办**：只能作废重建，没有"找回"

**配套陷阱：云存储桶按账号归属**。COS 桶名尾号 = 持有账号的 APPID（如 `aibao-audio-hk-1356733768` ↔ APPID `1356733768`）。换桶时，访问密钥也必须换到同一个账号的密钥——不同账号的密钥访问会 403。

**何时引入**：Plan 10 Day 2 修 COS 上传"三层洋葱"故障时（网络层 → 权限层 → 操作层）。

## 即将引入

- **双层安全链路**（前置 PreCheck + 后置 PostCheck，技术架构第 7 章）
- **强约束 System Prompt 模板**
- **真实 IP 同人化归一化**
- **签名 URL（Signed URL）**：私有资源临时放行，URL 内带签名和过期时间
- ✅ **JWT 鉴权** —— 见 10.7
- ✅ **手机号 hash + 加密双存** —— 10.4 hash + 10.10 AES-GCM
- **HTTPS 强制 + Let's Encrypt**
- **儿童数据合规**：COPPA / GDPR-K / 中国未保条例 / UK AADC
- **数据保留期**：用户注销后 30 天内删除
- **导出与删除接口**（合规要求）
- **预算熔断**：每日 LLM token 用量超阈值停服，防止半夜烧钱
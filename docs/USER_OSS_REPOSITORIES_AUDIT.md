# 每用户对象存储库审计（PR #5）

审计对象：`cursor/user-oss-repositories-23b1`（draft PR「Add per-user object storage repositories for generation output」）。对照 Grsai「文件仓库」行为，以及本仓库管理员多云存储里“解密失败不得把密钥写成空”的做法。实现方按不可信代码审查，结论以当前分支为准。

## 结论

可以进入 review。租户隔离、密钥落库和“没带 `oss-id` 就走原来的管理员存储”这几条主路径是成立的。审计里要修的问题已经补在同一分支上：用户填的端点不能把网关拨到内网或云元数据，返回给调用方的地址必须落在该用户配置的域名上，视频创建如果写不进 `oss-id` 绑定就不要先回成功。没有仍开放的阻断项。

## 阻断项

无。

没有发现用户 A 能读出或使用用户 B 的存储库，也没有发现 GET 回传密钥明文，或更新时空密钥把已存密文擦掉。

## 应修

下列项已在本分支修好。

1. **用户端点可以打到内网。** 任意登录用户都能保存 S3 endpoint、阿里云 region、Cloudflare account id。account id 和 region 会拼进主机名，`https://127.0.0.1`、`metadata.google.internal` 这类地址会被 HeadBucket / PutObject 打到网关所在网络。管理员存储允许私网 MinIO，是因为只有管理员能填；每用户仓库不能照搬。现在 endpoint 必须是公网 HTTPS、不能带用户信息、query、fragment 或路径；region 只允许主机名字符；Cloudflare account id 必须是 32 位十六进制。实际拨号会先解析 DNS，任一结果落在回环、私网、链路本地或元数据网段就拒绝。管理员存储不走这条限制。
2. **视频转存会跟着上游 URL 下载。** `oss-id` 存在时，Grok / Seedance 完成体会被网关自己 GET，再上传到用户桶。原先只判断 `http(s)://` 前缀，也跟随重定向，能绕过 Grok 内容代理对 `vidgen.x.ai` 和 `169.254.169.254` 的限制。现在只下载 HTTPS，拒绝用户信息、元数据主机和私网地址，最多跟随一次重定向，且每一跳都再检查。
3. **返回 URL 用字符串前缀比较。** `https://cdn.example` 会放过 `https://cdn.example.evil/...`。图片和视频现在按 scheme、主机、端口和路径前缀比较。域名里的 userinfo、query、fragment、私网主机也不能保存。
4. **`oss-path` 能把 `?`、`#`、`%` 写进对象键，从而改掉调用方看到的 URL。** 前缀只保留字母、数字、`.`、`_`、`-` 和 `/`，并拒绝 `.`、`..` 和空段。
5. **创建视频时绑定写失败会静默退回平台存储。** 响应先写出，Redis 写入在后面，失败只打日志。状态轮询又没带 `oss-id` 时，就会走管理员内容代理。现在在写成功响应之前保存绑定；保存失败则这次创建不算成功。状态轮询仍用创建时的用户和 API key 读取绑定。
6. **检测接口把 SDK 错误原文回给浏览器。** HeadBucket 失败时 `err.Error()` 可能带上请求细节。现在 4xx 配置错误仍返回我们自己的说明，其它失败只返回 `connection failed`。
7. **删用户不会带走存储库。** 外键没有 `ON DELETE CASCADE`，和 passkey 等用户数据不一致，硬删除用户会被挡住或留下密文。迁移已改为级联删除。
8. **检测接口没有单独的重查询限制。** 用户路由本来就有面板全局限流。检测会打到外部对象存储，现已再套一层 heavy 限流。

## 建议

这些不挡合并。

- `user_oss_repositories` 没有仓库层 SQL 测试。隔离和密文行为覆盖在 service / handler 的内存实现上。
- 每个用户最多 20 条是先读后写，没有事务，并发创建可以略超过上限。
- 检测仍可对公网桶做连通性探测，只是被 heavy 限流挡住高频调用。
- 自定义域名允许 `http`。网关不会去拉取这个域名；真正出站的 endpoint 和产物下载必须是 `https`。
- 自建、只在内网可达的 MinIO 不能作为每用户仓库。站点级管理员存储仍可以。
- 访问密钥 ID 会在列表和编辑表单里返回，密钥本身不会。这是编辑所需要的，不是密钥材料。
- 域名留空时，结果是存储端点上的预签名 URL，不是自定义域名。和“配置了域名才用该域名”一致。
- 管理端图片存储在解密失败时仍会把旧值当明文继续用。每用户仓库更严：解密失败直接中止，不改行。这次没有改管理端那条兼容逻辑。
- `/oss` 的浏览器点击没有在这轮重跑。Vue 组件测试原先已覆盖列表、新建、检测、编辑留空密钥和复制 id。

## 已对齐

- **产品。** `/oss` 可列表、新建、编辑、删除、检测、复制 id。提供商为 `aliyun`、`tencent`、`qiniuyun`、`cloudflare`、`s3`。`/docs/oss` 给出 `oss-id` 与可选 `oss-path`。没带 `oss-id` 时同步图、异步图、Grok 视频和 Seedance 仍走原来的管理员或默认存储。
- **租户。** 列表、读取、更新、删除、检测和 `oss-id` 解析都带 `user_id`。另一用户的 id 得到 not found，不区分“不存在”和“别人的”。生成请求用的是 API key 对应的登录用户，不是调用方自己填的用户 id。
- **密钥。** 只存密文。加密密钥未配置时拒绝写入。GET 只有 `secret_configured`。更新时密钥留空或只有空白会先解密再原样加密回去；加载失败或解密失败都不会执行 UPDATE。
- **生成路径。** 同步图片在非流式响应里转存，流式加 `oss-id` 会直接 400。异步任务把转存推迟到完成，失败则任务失败，不会退回管理员桶。Grok 视频和 Seedance 在完成体里上传 `video.url` / `content.video_url` / `video_url`。图片响应里没有可上传的图时也会失败，而不是把上游 body 原样返回。
- **表结构。** `241_user_oss_repositories.sql`：主键即 id，软删除保留行所以 id 不会被下一条配置复用；`user_id` 部分索引；外键级联删除用户。

## 验证

```text
go test -tags=unit ./internal/service/ -run 'TestUserOSS|TestImageTaskUserOSS|TestNormalizeOSS|TestPersistUserOSS'
go test -tags=unit ./internal/handler/ -run 'TestUserOSS'
go test -tags=unit ./internal/repository/ -run 'TestS3|TestImageStorage'
go test -tags=unit ./internal/server/routes/ -run TestDoesNotExist
```

以上均通过。前端文件没有改。

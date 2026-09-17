# workbuddy-cliproxy

## 0.2.1：视觉、思考配置与运行中断修复

- DeepSeek V4.1 Flash 注册 `text/image` 输入能力；图片内容完整透传。
- DeepSeek 模型注册 `low/high/max` 思考等级，未指定时默认 `reasoning_effort=high`；显式传入的 `reasoning_effort` 或 `thinking` 保留。hy3 保持历史的强制 high 行为。其他模型的未验证思考等级不作推测。
- CodeBuddy 要求第一条消息为 system（否则 HTTP 400、业务码 `11128`、`first message is not system prompt`）。插件为缺失 system 的对话补充默认 system，并将首条 developer 转为 system。
- HTTP 错误通过插件 ABI 保留真实状态码；流式响应在上游响应头校验成功后才开始。连接错误和 HTTP 502/503/504 在响应开始前最多尝试 3 次；400/401/429 不在插件内盲目重试，交由 CPA 处理。已输出的流不重放，避免重复执行工具。
- 聊天流取消原有 120 秒整轮超时；读取失败和上游错误事件向 CPA 报错，避免被当作正常结束。认证和配额请求仍保留超时。

### CPA 与 DSH 的能力配置

旧版 CPA `/v1/models` 只返回 id/object/created/owned_by，即使插件注册能力也会被过滤。本次同时修复 CPA 的 OpenAI 模型列表，保留 `thinking`、`supported_input_modalities`、`supported_output_modalities`、`supported_parameters` 和上下文/输出长度。仅安装插件到未修复的 CPA，模型列表仍可能缺少这些字段。

当前 DSH 自定义提供方的模型发现只读取名称和长度，不导入图片与思考能力。请在已有 `$DSH_HOME/settings.yaml` 的对应提供方下合并以下字段（`your-cpa-provider` 替换为现有 Provider ID；保留已有地址和凭据配置）：

```yaml
llm-pi-ai:
  providers:
    your-cpa-provider:
      models:
        - id: deepseek-v4.1-flash
          input: [text, image]
          reasoningEfforts:
            low: low
            high: high
            max: max
          compat:
            thinkingFormat: openai
            supportsReasoningEffort: true
```

不要覆盖整个 models 列表；仅修改 DeepSeek 对应项。目录提供方请将对应模型配置放在 `modelOverrides.deepseek-v4.1-flash` 下（省略 id）。DSH 配置变更在下一次请求生效。

在线验证：`CPA_API_KEY=<API密钥> node smoke.mjs http://127.0.0.1:8317`。脚本检查模型元数据，并实际调用普通、流式、内嵌红色 PNG 三种请求，会消耗少量模型额度。测试不带 system 的请求同时验证 `11128` 修复和默认思考。

## 国内 / 国际双入口（0.2.0）

- `workbuddy-cn.so`：国内站 OAuth、认证文件与请求转发。
- `workbuddy.so`：国际站 OAuth、认证文件与请求转发。
- 分别构建：`make build PLUGIN_ID=workbuddy-cn VERSION=0.2.0` 和 `make build PLUGIN_ID=workbuddy VERSION=0.2.0`。将两个文件安装到 CPA 插件目录，并启用两个同名配置项。
- 旧凭据没有 `workbuddy_provider` 字段时按国内站处理；`auth.domain` 包含 `codebuddy.ai` 时按国际站处理。新凭据保存明确的版本标识。已有文件名保持不变。
- 统一配额页由 `workbuddy` 国际插件提供，同时展示两类账号，支持筛选、中英文、深色模式、手动刷新和每 60 秒自动刷新。
- 页面数据和管理密钥仅存储于当前标签页的 `sessionStorage`，刷新页面会保留；关闭标签页后清除。点击「清除缓存」同时移除密钥与数据。刷新失败保留旧数据并显示错误。

以下历史安装说明中的单插件部署应按上述双入口配置调整。

把**腾讯 CodeBuddy**（`copilot.tencent.com`）封装成 [CLIProxyAPI](https://github.com/router-for-me/CLIProxyAPI)(CPA)插件,任何支持 OpenAI / Anthropic 协议的客户端(Claude Code、Cursor、Cline、SDK……)都能直接调用 CodeBuddy 背后的模型。

对 [Sliverkiss/cpa-plugin](https://github.com/Sliverkiss/cpa-plugin) 公开 `workbuddy.so` 的 clean-room 逆向重写,补齐了源码与 x86_64 支持;workbuddy 的原始设计归属 Sliverkiss。

## 工作原理

在 CPA 里注册为 `workbuddy` provider:负责 CodeBuddy 扫码登录、token 刷新,并把请求转发到 `copilot.tencent.com/v2/chat/completions`。登录后按账号保存为独立的 `workbuddy-<hash>.json`,支持多个账号共存。旧版 `workbuddy.json` 仍可读取和刷新;旧版已被覆盖的账号需要重新授权。

## 模型

`glm-5.2` · `glm-5.1` · `glm-5v-turbo` · `kimi-k2.7` · `minimax-m3-pay` · `hy3` · `hy3-preview` · `hy3-preview-agent` · `hy4-preview` · `gpt-6-astra` · `deepseek-v4-pro` · `deepseek-v4-flash` · `deepseek-v4.1-flash`

国际版 `workbuddy` 另支持 `glm-5.3`（GLM-5.3），思考强度为 `low` / `high` / `max`，未指定时默认 `high`；显式 `reasoning_effort` 或 `thinking` 设置保持不变。默认上下文为 300,000 token，支持扩展到 1,000,000 token；模型元数据声明默认值 300,000，输出上限沿用 8192 token。

国内和国际插件均声明以下模型支持 `text/image` 输入：`glm-5.2`、`glm-5v-turbo`、`kimi-k2.7`、`minimax-m3-pay`、`hy3`、`deepseek-v4-pro`、`deepseek-v4-flash`、`deepseek-v4.1-flash`。能力依据 [WorkBuddy 官方模型列表](https://www.codebuddy.cn/docs/workbuddyapp/features/Model) 和已有图片验证；图片 URL、内嵌 base64 与 detail 参数完整透传。输出仍声明为文本，图片输入不代表图片生成、音频或视频能力。

未确认的模型与预览变体暂不增加图片能力声明；不根据系列名前缀推断。具体可用性以 CodeBuddy 账号权限为准。客户端若不导入模态字段，仍需在客户端配置对应模型的 `input: [text, image]`。

Hy4 Preview 和 GPT-6-Astra 的请求模型 ID 分别为 `hy4-preview` 和 `gpt-6-astra`，国内、国际插件均注册。上下文上限暂不声明，输出上限沿用插件的 8192 token 配置。

## 安装

**前置**:运行中的 CLIProxyAPI v7.2.x(带 CGO / 插件支持)、CodeBuddy 账号。编译架构需与 CPA 实例一致(amd64 / arm64)。Docker 部署必须把宿主 `plugins/` 挂到容器 `/CLIProxyAPI/plugins`,否则重启会丢插件。

CPA 按 `plugins/<goos>/<goarch>/` 再回落到 `plugins/` 查找动态库,Docker linux/amd64 推荐放到 `plugins/linux/amd64/workbuddy.so`。

### 从 GitHub Release 安装(推荐)

打 `v*` 标签后,Actions 会发布符合 [CLIProxyAPI 插件商店](https://github.com/router-for-me/CLIProxyAPI-Plugins-Store) 规范的产物:

| 文件 | 内容 |
|------|------|
| `workbuddy_<version>_linux_amd64.zip` | `workbuddy.so` |
| `workbuddy_<version>_linux_arm64.zip` | `workbuddy.so` |
| `workbuddy_<version>_darwin_amd64.zip` | `workbuddy.dylib` |
| `workbuddy_<version>_darwin_arm64.zip` | `workbuddy.dylib` |
| `workbuddy_<version>_windows_amd64.zip` | `workbuddy.dll` |
| `checksums.txt` | 各 zip 的 SHA256 |

```bash
# 以 linux/amd64 为例;把 <version> 换成 Releases 里的版本号(不带 v)
# https://github.com/zhujiane/workbuddy-cliproxy/releases
unzip workbuddy_<version>_linux_amd64.zip
mkdir -p /path/to/CLIProxyAPI/plugins/linux/amd64
cp workbuddy.so /path/to/CLIProxyAPI/plugins/linux/amd64/
```

Docker Compose(官方 compose 已挂载 `./plugins`):

```bash
cd /path/to/CLIProxyAPI
unzip workbuddy_<version>_linux_amd64.zip
mkdir -p plugins/linux/amd64
cp workbuddy.so plugins/linux/amd64/
```

`config.yaml` 启用:

```yaml
plugins:
  enabled: true
  dir: "plugins"
  configs:
    workbuddy: { enabled: true, priority: 100 }
```

重启 CPA:

```bash
docker compose restart cli-proxy-api
# 或 docker restart cli-proxy-api
```

日志出现 `plugin loaded ... plugin_id=workbuddy` 即成功,`GET /v1/models` 也能看到上面的模型。然后到 CPA 面板添加 workbuddy 凭据,扫码登录 CodeBuddy。

### 从源码编译

需要 Go 1.26+ 与 gcc。

```bash
git clone https://github.com/zhujiane/workbuddy-cliproxy.git
cd workbuddy-cliproxy
make build                          # 当前平台
# 或指定目标: make build GOOS=linux GOARCH=amd64
```

装进已挂载的 CPA 插件目录并重启容器:

```bash
make install PLUGIN_DIR=/path/to/CLIProxyAPI/plugins RESTART_DOCKER=cli-proxy-api
```

等价手搓:

```bash
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
  go build -buildmode=c-shared -ldflags "-s -w -X main.pluginVersion=0.1.0" \
  -o workbuddy.so .
```

### GitHub Actions 自动构建

| 触发 | 工作流 | 产物 |
|------|--------|------|
| push / PR 到 `dev`、`main` | [ci.yml](.github/workflows/ci.yml) | 跑测试,并上传 `linux/amd64` zip(Actions Artifacts,保留 14 天) |
| 推送标签 `vX.Y.Z`(如 `v0.1.0`) | [release.yml](.github/workflows/release.yml) | 多平台 zip + `checksums.txt`,发布到 GitHub Release |
| 手动 `workflow_dispatch` | `release.yml` | 同样打多平台包,但**不**发 Release(只当快照 Artifact) |

发正式版:

```bash
git tag v0.1.0
git push origin v0.1.0
```

标签必须是 `v` + 数字开头版本(`v0.1.0`),CPA 插件商店用它当插件版本。zip 内必须是根目录下的 `workbuddy.so` / `.dylib` / `.dll`,不要套一层文件夹。

## 使用

CPA 默认端口 `8317`,API key 见 `config.yaml` 的 `api-keys`。

| 协议 | Base URL |
|------|----------|
| OpenAI | `http://<host>:8317/v1` |
| Anthropic | `http://<host>:8317`(不带 `/v1`,走 `x-api-key`) |

```bash
# Claude Code
export ANTHROPIC_BASE_URL=http://localhost:8317
export ANTHROPIC_API_KEY=<api-key>
export ANTHROPIC_MODEL=hy3-preview-agent
claude
```

```bash
# curl / OpenAI
curl http://localhost:8317/v1/chat/completions \
  -H "Authorization: Bearer <api-key>" -H "Content-Type: application/json" \
  -d '{"model":"hy3-preview-agent","messages":[{"role":"user","content":"你好"}],"stream":true}'
```

流式 / 非流式都支持;非流式请求会被内部转成流式再聚合(CodeBuddy 上游 `code 11101` 拒绝非流式)。

## Claude Code 兼容性

腾讯 CodeBuddy 的内容审核把 Claude Code 的两句固定 system 模板逐字加进了黑名单,命中即回"敏感内容"拒答:

- `You are Claude Code, Anthropic's official CLI for Claude.`(身份句)
- `Main branch (you will usually use this for PRs)`(git 注入句)

任何一字改动都绕过(精确匹配,非语义审核)。workbuddy 转发前会自动把这两句做最小改写(`CLI`→`CLI tool`、`Main branch`→`Default branch`),语义不变,Claude Code 照常工作。

属于 cat-and-mouse:腾讯哪天多加模板句,得跟着改 `sanitizeBlockedTemplates`。

## 思考模式

hy3 系列(`hy3` / `hy3-preview` / `hy3-preview-agent`)自动开最大思考:workbuddy 转发前强制 `reasoning_effort=high`,覆盖客户端任何设置。CodeBuddy 只对 `high` 真正开深度思考(`medium` / `max` / `xhigh` 等档位它直接忽略),所以这已是 hy3 能用的最高档。思考内容走 SSE 的 `delta.reasoning_content`,客户端要支持渲染思考块才看得到。

## 流式

真流式(async):转发上游时边读边通过 `host.stream.emit` 把每个 chunk 实时推给 CPA,客户端逐字收到(不是等收齐了一股脑)。hy3 几千字的思考过程也是实时流出的,不是憋半天再刷出来。

## License

MIT。

## 配额与账号信息

更新插件并重启 CPA 后，从侧边栏 **WorkBuddy 配额** 打开页面；也可以访问
`/v0/resource/plugins/workbuddy/panel`。输入 CPA **管理密钥**，点击「查询 / 刷新」，
按账号查看昵称、UID、邮箱（上游提供时）、企业 ID，以及剩余 / 已用积分、资源包和周期结束时间。
密钥仅保存在当前标签页会话存储中，不写入持久化 localStorage。

管理接口：`GET /v0/management/plugins/workbuddy/credits`，使用
`Authorization: Bearer <管理密钥>`。可添加 `?auth_index=<账号索引>` 查询单个账号。
返回 `accounts` 数组；查询失败的账号返回 `error`，不会伪装成零余额。

积分统计来自 CodeBuddy **个人资源包**计费接口，优先使用当前周期数据，分页汇总有效和已耗尽资源包。
不代表企业共享配额，也不代表所有模型分别可用的额度；计费数据可能延迟。
接口协议参考 [Sliverkiss/cpa-plugin 的计费实现](https://github.com/Sliverkiss/cpa-plugin/blob/main/workbuddy/billing.go)。

账号资料会映射到 CPA 凭据的标签和备注。旧凭据已有的昵称会在重新加载时显示；
如果旧文件在登录时未成功保存账号资料，需要重新扫码授权补齐。
新登录会等待账号信息读取成功后再保存，避免生成只有随机 ID 的凭据。

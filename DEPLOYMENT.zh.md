# 2026-09-17 国际版聊天域名修复

国际插件的三条聊天路径（非流式、异步流式、同步流式）改为请求 `https://www.workbuddy.ai/v2/chat/completions`，与官方 WorkBuddy 客户端一致。登录、凭据刷新和国内插件保持原配置。旧 CodeBuddy 聊天入口对 GPT-6-Astra 返回 11102。

同时将带 `choices` 的上游流事件中 `object: response` 规范为 `chat.completion.chunk`，避免 CPA Responses 转换器丢弃事件、触发空流重试。原生 Responses 对象不受影响，新增回归测试覆盖内容、结束原因和用量保留。

`go test ./...` 和国际插件构建通过，已部署 `workbuddy.so` 并重启 `cli-proxy-api`。旧国际插件备份：`/root/work/CLIProxyAPI/backups/workbuddy-chat-domain-dB4I5ntW/workbuddy.so`。

部署后通过 CPA `/v1/responses` 实测 `gpt-6-astra` 流式请求：HTTP 200，输出 `OK`，收到 `response.completed`。

# 2026-09-17 国内版 GLM-5.3 支持

已将国内插件 `workbuddy-cn` 增加 `glm-5.3-flash` 与 `glm-5.3`，并重新构建国际插件保持同一版本 `0.2.2`。两个模型默认上下文为 300K，支持 `low` / `high` / `max` 推理档位，未指定时插件补 `high`；Flash 声明 `text/image` 输入，完整 GLM-5.3 声明文本输入。已用国内账号直接请求两个模型，并通过 CPA 发送 Flash 图片请求验证成功。

部署位置：`/root/work/CLIProxyAPI/plugins/linux/amd64/`。旧动态库备份在 `/root/work/CLIProxyAPI/backups/glm53-domestic-20260917-020029/`，容器 `cli-proxy-api` 已重启并加载 `workbuddy-cn` / `workbuddy` 版本 `0.2.2`。CPA 当前 `/v1/models` 能看到 `glm-5.3-flash` 的国内 provider 条目；同名 `glm-5.3` 会由 CPA 按可用账号池选择对应 provider。

# 2026-09-15 修复记录

## 根因

CPA 日志中 DeepSeek V4.1 Flash 的原始错误是 HTTP 400，CodeBuddy 业务码 `11128`，消息 `first message is not system prompt`。插件此前没有为不带 system 的请求补齐上游要求的首条消息，又把 HTTP 错误变成没有状态码的插件错误；CPA 最终展示 `503 auth_unavailable`，导致 DSH 报本轮失败。账号登录本身在排查时正常。

视觉能力缺失涉及模型元数据：插件没有声明输入模态，CPA 的 OpenAI 模型列表过滤掉能力字段，而当前 DSH 模型发现只导入名称和长度。上游图片能力仍正常，内嵌纯红 PNG 实测返回 Red。

## 部署

- 两个插件：`workbuddy` / `workbuddy-cn`，版本 0.2.2。
- CPA：基于原运行版本 v7.2.157 的提交 `09a29bd`，只修改模型列表序列化与字段过滤。未用工作目录的新版本整体替换旧版本。
- CPA 镜像：`local/cpa-workbuddy:7.2.157-0.2.1`。
- 镜像选择：`/root/work/CLIProxyAPI/docker-compose.override.yml`；常规 `docker compose up -d` 会自动使用该文件。本地修复镜像设置 `pull_policy: never`。如显式使用 `-f`，须同时包含 override 文件。
- 原插件、原 CPA 二进制与新镜像构建文件：`/root/work/CLIProxyAPI/backups/workbuddy-fix-20260915-nRpOuo/`。
- 原版本隔离源码：`/tmp/cpa-workbuddy-7.2.157`。可移植补丁保存在本仓库 `patches/cpa-v7.2.157-model-capabilities.patch`。
- 当前 `/root/work/CLIProxyAPI` 工作目录也保留同等源码修复与回归测试，供后续合并使用。未提交或推送 Git。

## 验证结果

- 插件 `go test -race ./...` 通过；国内/国际两个共享库编译通过。
- CPA registry 和 OpenAI handlers 测试通过，服务器编译通过。
- 部署后 `/v1/models` 返回 DeepSeek 的 `text/image`、`reasoning_effort`、`thinking.levels=[low,high,max]`。
- 无 system 的非流式请求：HTTP 200、OK、有 reasoning_content。
- 无 system 的流式请求：HTTP 200、OK、有 reasoning_content。
- 内嵌红 PNG：HTTP 200、Red、有 reasoning_content。
- 故意发送非法空 messages：HTTP 400，保留上游 11128 和完整原因，不再变成误导性的 503。

DSH 实例的 settings.yaml 未在本机找到，尚未修改客户端设置。README 提供应合并到已有模型项的 input / reasoningEfforts / compat 配置；单独刷新 DSH 模型列表不足以导入这些能力。

## 回滚与后续升级

需要回滚时，先停止 cli-proxy-api 服务，将备份目录中的两个旧插件复制回 `plugins/linux/amd64/`，将 compose override 文件改名保留，然后使用原 docker-compose.yml 启动原镜像。原镜像仍在本机，原配置和凭据未修改。

升级到已包含同等修复的官方 CPA 后，应移除本地 override 的镜像固定配置，避免继续运行该本地补丁版。不要仅删除 override 就认为插件能力字段在旧官方 CPA 中仍会显示。

# 2026-09-15 修复记录

## 根因

CPA 日志中 DeepSeek V4.1 Flash 的原始错误是 HTTP 400，CodeBuddy 业务码 `11128`，消息 `first message is not system prompt`。插件此前没有为不带 system 的请求补齐上游要求的首条消息，又把 HTTP 错误变成没有状态码的插件错误；CPA 最终展示 `503 auth_unavailable`，导致 DSH 报本轮失败。账号登录本身在排查时正常。

视觉能力缺失涉及模型元数据：插件没有声明输入模态，CPA 的 OpenAI 模型列表过滤掉能力字段，而当前 DSH 模型发现只导入名称和长度。上游图片能力仍正常，内嵌纯红 PNG 实测返回 Red。

## 部署

- 两个插件：`workbuddy` / `workbuddy-cn`，版本 0.2.1。
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

# AI PR Review 项目指南

## 构建与运行

```sh
go build -o ai-pr-review ./cmd/ai-pr-review
```

## 集成验证

本地密钥配置在 `.ai-pr-review/settings.local.json`（已 gitignore，不进仓库）。

你可以通过以下命令行在一个真实的环境中进行测试：

```sh
# 从本地配置加载密钥到环境变量
export ANTHROPIC_API_KEY=$(jq -r '.apiKey' .ai-pr-review/settings.local.json)
export ANTHROPIC_BASE_URL=$(jq -r '.baseURL' .ai-pr-review/settings.local.json)
export ANTHROPIC_MODEL=$(jq -r '.model' .ai-pr-review/settings.local.json)

# 用真实 PR 做端到端验证
./ai-pr-review --pr https://github.com/kubernetes/kubernetes/pull/139381 --format markdown
```

## 提交与 PR 流程

**禁止直接 push 到 master 分支。** 任何修改都必须走分支 + PR 流程。

### 1. 创建分支

分支名采用约定式提交格式（小写，英文，连字符分隔）

### 2. 提交代码

### 3. 推送并创建 PR

```sh
git push -u origin <branch-name>
gh pr create \
  --title "feat: add PR review cache layer" \
  --body "$(cat <<'EOF'
## 功能描述
<!-- 说明该功能的作用与使用方式 -->

## 实现思路
<!-- 简要说明技术选型或核心实现逻辑 -->

## 测试方式
<!-- 如何验证该功能正常运行 -->
EOF
)"
```

### PR 规范

- **标题**：约定式提交（英文），如 `feat: add diff parser`、`fix: handle nil token`
- **内容** 必须包含三个部分：
  - **功能描述**：说明该功能的作用与使用方式
  - **实现思路**：简要说明技术选型或核心实现逻辑
  - **测试方式**：如何验证该功能正常运行
- 在 GitHub PR 页面手动合并（不直接 push 到 master）

## 项目结构

```
cmd/ai-pr-review/     CLI 入口
internal/
  api/                LLM API 客户端（Anthropic / OpenAI / 多 provider）
  auth/               认证与凭证管理
  config/             配置加载（三层叠加：全局 → 项目 → 本地）
  pr/                 GitHub PR 获取、URL 解析、Diff 解析
  review/             Review 引擎（分类器、引擎、输出格式化）
  runtime/            会话管理、对话循环
  tui/                终端 UI
  tools/              AI 可用工具（文件读写、搜索等）
```

## 新增功能后的自检清单

1. 为新代码写 `*_test.go`，覆盖正常路径和错误路径
2. `go test ./...` 全部通过
3. 用真实 PR URL 跑一次 `--format markdown` 确认输出可读
4. 用真实 PR URL 跑一次 `--format json` 确认 JSON 可被 `jq` 解析
5. 创建分支 → 提交 → 推送 → 创建 PR（标题用英文约定式提交，内容包含功能描述/实现思路/测试方式）

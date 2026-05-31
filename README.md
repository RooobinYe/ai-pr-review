# AI PR Review 助手

<p align="center">
  <strong>题目三：AI PR Review 助手</strong><br/>
  一个工程级 AI 代码评审工具，可插入 CI/CD 流水线作为质量门禁。
</p>

<p align="center">
  <a href="#-demo-视频">📺 Demo 视频</a> •
  <a href="#快速开始">🚀 快速开始</a> •
  <a href="#cicd-集成">🔧 CI/CD 集成</a> •
  <a href="#项目结构">📁 项目结构</a>
</p>

## 📺 Demo 视频

[![Demo 视频](https://img.shields.io/badge/Bilibili-演示视频-00A1D6?style=for-the-badge&logo=bilibili)](https://www.bilibili.com/video/BV1BRVD6yEpQ/)

## 项目简介

在日常开发中，Pull Request Review 往往成为团队协作的瓶颈——Reviewer 需要花费大量时间理解代码变更、识别潜在风险、给出建设性建议。AI PR Review 助手通过 AI 辅助分析，自动化 PR 评审中的重复性工作，让 Reviewer 聚焦于最关键的设计决策和逻辑正确性。

### 与其他 PR Review 工具的本质区别

市面上大多数 AI PR Review 工具只能输出非结构化文本，无法融入工程流水线。本项目的核心差异在于**原生支持 CI/CD 集成**：

- `--format json` 输出带 schema 版本号的结构化 JSON，下游工具可直接消费
- `--quiet` 确保 stdout 只有纯 JSON，可直接管道给 `jq` 或 CI 插件
- `--fail-on` 作为质量门禁，当检测到指定严重级别以上的问题时以非零退出码阻断合并
- 即使在出错时也输出结构化 `ErrorOutput` JSON，CI 永远不会收到空响应

### 核心功能

| 功能 | 说明 |
|------|------|
| **PR 变更总结** | 自动获取 PR 的 diff 代码，生成结构化的变更摘要，包括变更范围、影响模块、核心逻辑变化 |
| **风险代码识别** | 基于全仓库上下文探索，识别安全漏洞、性能问题、并发风险、空指针、错误处理缺陷等 |
| **Review 建议生成** | 针对识别出的问题，生成具体、可操作的 Review 建议，包括问题描述、严重程度、置信度、修复方案 |
| **双模式运行** | 交互式 TUI（开发者本地使用）和 非交互 `--format` 模式（嵌入 CI/CD 自动运行） |
| **多 Provider 支持** | Anthropic Claude、OpenAI GPT 系列，可灵活切换 |

## 快速开始

### 前置条件

- Go 1.24+
- GitHub Personal Access Token（用于访问 PR 信息，公开仓库可不设但受严格频率限制）
- Anthropic API Key 或 OpenAI API Key

### 安装

```sh
git clone https://github.com/yezhenrong/ai-pr-review
cd ai-pr-review
go build -o ai-pr-review ./cmd/ai-pr-review
```

### 评委直接下载

评委可直接在 Github Release 页面下载预编译二进制，开箱即用，无需配置 API Key。

### 基本使用

```sh
# 交互式 TUI 模式：评审 PR 并可追问
./ai-pr-review --pr https://github.com/RooobinYe/ai-pr-review/pull/15

# Markdown 格式输出（适合人阅读）
./ai-pr-review --pr https://github.com/RooobinYe/ai-pr-review/pull/15 --format markdown

# JSON 格式输出（适合 CI/CD 消费）
./ai-pr-review --pr https://github.com/RooobinYe/ai-pr-review/pull/15 --format json

# 用中文评审，并指定模型
./ai-pr-review --pr https://github.com/RooobinYe/ai-pr-review/pull/15 --format markdown 使用中文回答，重点关注安全问题
```

### 环境变量

| 变量 | 说明 | 必填 |
|------|------|------|
| `GITHUB_TOKEN` | GitHub Personal Access Token | 推荐 |
| `ANTHROPIC_API_KEY` | Anthropic API Key | 二选一 |
| `OPENAI_API_KEY` | OpenAI API Key | 二选一 |
| `ANTHROPIC_MODEL` | 模型选择（默认 `claude-sonnet-4-20250514`） | 否 |
| `ANTHROPIC_BASE_URL` | API Base URL（支持自定义代理） | 否 |

### 配置文件（三层叠加）

| 优先级 | 路径 | 用途 |
|--------|------|------|
| 低 | `~/.ai-pr-review/settings.json` | 用户全局配置 |
| 中 | `.ai-pr-review/settings.json` | 项目级配置 |
| 高 | `.ai-pr-review/settings.local.json` | 本地密钥覆盖（已 gitignore） |

## CI/CD 集成

### 工程级特性

这是本项目与其他项目最本质的区别。

#### 1. 结构化 JSON 输出

```sh
./ai-pr-review --pr https://github.com/RooobinYe/ai-pr-review/pull/15 --format json --quiet
```

输出包含 `schema_version`（当前为 2）、`tool_version`、`model`、`generated_at`、PR 元信息、分类后的文件变更列表、以及分为 `must_fix` 和 `points_of_interest` 两个数组的风险发现：

```json
{
  "schema_version": 2,
  "tool_version": "1.0.0",
  "model": "claude-sonnet-4-20250514",
  "generated_at": "2026-05-31T12:00:00Z",
  "pr_info": { "owner": "owner", "repo": "repo", "pull_number": 123 },
  "title": "feat: add cache layer",
  "author": "developer",
  "file_changes": [
    { "filename": "cache/redis.go", "category": "code", "additions": 45, "deletions": 3 }
  ],
  "summary": "This PR introduces a Redis caching layer...",
  "must_fix": [
    {
      "file": "cache/redis.go",
      "line": 42,
      "severity": "critical",
      "confidence": "high",
      "category": "security",
      "title": "Redis connection without auth",
      "evidence": "redis.NewClient(&redis.Options{Addr: addr})",
      "description": "The Redis client is created without password authentication.",
      "suggestion": "Set Password field in redis.Options from environment variable."
    }
  ],
  "points_of_interest": [],
  "warnings": []
}
```

#### 2. `--fail-on` 质量门禁

```sh
# 如果 must_fix 中有 critical 或 high 级别的问题，退出码为 5
./ai-pr-review --pr $PR_URL --format json --fail-on high
```

| 退出码 | 含义 |
|--------|------|
| 0 | 成功，未命中门禁阈值 |
| 1 | 配置/凭证错误 |
| 2 | AI/API 调用失败 |
| 4 | 参数使用错误 |
| 5 | `--fail-on` 阈值命中（质量门禁未通过） |

#### 3. 错误输出也是 JSON

即使在出错时（凭证缺失、API 故障、AI 返回异常），stdout 也始终输出合法的 JSON，CI 管道永远不会收到空响应：

```json
{
  "schema_version": 2,
  "tool_version": "1.0.0",
  "generated_at": "2026-05-31T12:00:00Z",
  "error": {
    "code": "NO_CREDENTIALS",
    "message": "no credentials found — pass --api-key, set ANTHROPIC_API_KEY, or run /login"
  }
}
```

#### 4. GitHub Actions 示例

```yaml
- name: AI PR Review
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
  run: |
    ./ai-pr-review \
      --pr "${{ github.event.pull_request.html_url }}" \
      --format json \
      --quiet \
      --fail-on high
```

#### 5. 构建时注入凭证（零配置二进制）

```sh
go build -ldflags "\
  -X 'main.DefaultAPIKey=sk-xxx' \
  -X 'main.DefaultBaseURL=https://api.example.com' \
  -X 'main.DefaultModel=claude-sonnet-4-20250514' \
  -X 'main.Version=1.0.0'" \
  ./cmd/ai-pr-review
```

这使得可在 CI 中直接分发单个二进制文件，无需环境变量或配置文件。

## 设计思路

### 双阶段 Agentic 循环

JSON 模式采用独特的**双阶段设计**：

1. **阶段一 - 上下文探索**：AI 获得完整的工具访问权限（`read_file`、`grep`、`glob`、`bash`），在克隆的仓库中自由探索——完整读取关键文件、追踪调用方、检查测试覆盖、查看 git 历史
2. **阶段二 - 结构化生成**：基于阶段一积累的上下文，AI 生成符合严格 JSON Schema 的结构化审查结果

这解决了单阶段提示词「既要探索又要结构化输出」的质量问题。

### 全仓库上下文 vs 仅 Diff

不同于仅分析 diff 的简单工具，本项目会**克隆 PR 的头部分支**到本地，让 AI 能够：
- 阅读变更文件的完整内容（不仅看 diff hunk）
- 追踪函数调用方和被调用方
- 检查现有测试覆盖
- 理解项目整体架构

### 反幻觉控制

系统提示词内建五条反幻觉规则：
1. **证据要求**：每个发现必须引用实际读取的代码
2. **不确定性标注**：推测性问题明确标记为低置信度
3. **禁止虚构**：不允许捏造函数名、文件路径或代码
4. **平衡检查**：对每个 critical/high 发现，考虑替代解释
5. **数量上限**：最多 15 个发现，强制区分信号与噪音

### 模型选择

支持多 LLM 后端，可根据任务复杂度灵活切换：
- **Anthropic Claude**（Opus 4.6 / Sonnet 4.6 / Haiku 4.5）
- **OpenAI**（GPT-4o / GPT-4o-mini / o1-mini）
- **AWS Bedrock** / **GCP Vertex AI** / **Azure AI Foundry**（Stub，可扩展）

### 上下文获取方式

- **结构化 Diff 解析**：将 Git unified diff 解析为 `DiffFile → DiffHunk → DiffLine` 三层结构，保留行号映射
- **文件分类器**：自动将变更文件归类为 `code | test | config | doc | other`，支持 30+ 语言生态
- **仓库克隆**：使用 go-git 纯 Go 实现克隆，无系统依赖

## 依赖说明

### Go 第三方依赖

| 依赖 | 用途 | 许可证 |
|------|------|--------|
| [go-git/go-git](https://github.com/go-git/go-git) v5 | 纯 Go Git 操作（克隆、检出） | Apache 2.0 |
| [mattn/go-runewidth](https://github.com/mattn/go-runewidth) | 终端字符宽度计算 | MIT |
| [rivo/uniseg](https://github.com/rivo/uniseg) | Unicode 分词 | MIT |
| [golang.org/x/sys](https://golang.org/x/sys) | 系统调用 | BSD-3 |
| [golang.org/x/term](https://golang.org/x/term) | 终端 I/O | BSD-3 |

### 外部服务

| 服务 | 用途 |
|------|------|
| GitHub API | 获取 PR 详情和变更文件列表 |
| Anthropic API / OpenAI API | AI 模型推理 |

## 项目结构

```
ai-pr-review/
├── cmd/ai-pr-review/        # CLI 入口（标志解析、流水线编排）
└── internal/
    ├── api/                 # LLM API 客户端（Anthropic / OpenAI / Bedrock / Vertex / Foundry）
    │   └── providers/       # 各 Provider 适配层
    ├── auth/                # 认证与多 Provider 凭证管理
    ├── compat/              # 兼容层（清单导出、会话重放等子命令）
    ├── config/              # 配置加载（三层叠加：全局 → 项目 → 本地）
    ├── context/             # 上下文组装（环境信息、git 状态、CLAUDE.md 注入）
    ├── permissions/         # 权限管理（4 种模式 + 规则集）
    ├── pr/                  # GitHub PR 获取、URL 解析、Diff 结构化解析、仓库克隆
    ├── prompt/              # Review 专用提示词构建（XML 结构、反幻觉规则）
    ├── review/              # Review 数据类型、文件分类器
    ├── runtime/             # 会话管理、Agentic 对话循环、配置加载
    ├── tools/               # AI 可用工具（bash、read_file、grep、glob 等 10 个）
    ├── tui/                 # 终端 UI（Bubble Tea 架构）
    └── usage/               # Token 用量追踪与成本估算
```

## 许可证

MIT

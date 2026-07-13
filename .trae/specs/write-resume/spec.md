# 比赛报名简历撰写 Spec

## Why
用户希望报名"挑战杯"比赛"基于多模态AI监测的老年人跌倒风险、心理健康、诈骗识别及预警研究"题目的**接口调用与系统集成**方向，需要基于当前未完成的 fake_tiktok 项目撰写项目经历简历，突出与该方向匹配的技术能力。

## What Changes
- 创建一份项目经历简历文档，聚焦接口调用与系统集成方向
- 简历需基于项目实际技术栈和已实现功能撰写，不夸大未完成部分
- 突出与比赛方向（萤石开放平台硬件对接、RESTful API、WebSocket、IoT设备集成、第三方平台SDK调用）相关的技术能力

## Impact
- 产出文件：`d:\study_project\fake_tiktok\resume_项目经历.md`
- 不影响任何现有代码

## ADDED Requirements

### Requirement: 项目经历简历
系统应生成一份结构化的项目经历简历文档，包含以下内容：

#### Scenario: 简历内容覆盖
- **WHEN** 用户提交简历用于比赛报名
- **THEN** 简历应包含以下板块：
  1. 项目概述（1-2句话概括项目定位和规模）
  2. 技术栈（突出与系统集成相关的技术）
  3. 核心职责（聚焦接口调用与系统集成方向）
  4. 项目亮点/技术难点（从项目中提炼与比赛方向匹配的能力点）
  5. 当前进展与后续规划（诚实说明项目状态）

#### Scenario: 技术能力匹配
- **WHEN** 评审阅读简历
- **THEN** 能清晰看到以下与"接口调用与系统集成"方向匹配的能力：
  - RESTful API 设计与开发（Gin 框架，20+ 接口）
  - WebSocket 实时通信（弹幕系统）
  - 多服务集成编排（MySQL/Redis/RabbitMQ/ES，Docker Compose）
  - 第三方服务对接（QQ SMTP 邮件、高德地图 API、验证码服务）
  - 消息队列异步集成（RabbitMQ 弹性连接+PublishBuffer）
  - 缓存与数据同步策略（Redis 多数据结构+Cache-Aside+Write-Through）
  - 容器化部署与健康检查（Docker 多阶段构建+Compose 编排）

#### Scenario: 诚实性要求
- **WHEN** 描述项目状态
- **THEN** 明确标注项目"开发中"，区分已完成和未完成功能，不夸大未实现部分

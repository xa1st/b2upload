# B2 图床上传工具 (b2upload) 📸

  Go Language
  MIT License
  GitHub stars (https://github.com/xa1st/b2upload)
  GitHub forks (https://github.com/xa1st/b2upload)
  GitHub issues (https://github.com/xa1st/b2upload/issues)
  license (https://github.com/xa1st/b2upload/blob/master/LICENSE)

  一款基于 Go 开发的轻量级命令行图床上传工具，专为 Backblaze B2 云存储服务设计，支持批量上传、多标签配置和并发处理，提供
  高效稳定的文件上传体验。

## ✨ 核心特性

  | 特性 | 说明 |
  | --- | --- |
  | 🚀 高性能并发 | 支持最多5个并发上传，大幅提升批量文件上传效率 |
  | 📁 灵活文件匹配 | 支持单文件、目录、通配符模式（如 *.png）多种输入方式 |
  | 🏷️ 多标签管理 | 支持多个图床配置标签，可灵活切换不同用户和域名 |
  | 🔐 安全认证 | 基于 Backblaze B2 官方API，支持Token和Bucket双重认证 |
  | 📊 实时反馈 | 显示上传进度、成功率、耗时统计，支持跳过已存在文件 |
  | 🛠️ 智能命名 | 自动生成基于MD5和时间的远程文件路径，避免冲突 |
  | 📋 TOML配置 | 使用简洁的TOML格式配置文件，支持多环境管理 |

## 🚀 快速开始

  ### 🔍 前提条件

  - 安装 Go 环境（推荐 1.25+）：通过 Go官网 (https://golang.org/dl/) 或包管理器安装
  - 拥有 Backblaze B2 账户和对应的API密钥
  - 操作系统：Linux、macOS、Windows（PowerShell推荐）

  ### 🛠️ 安装与运行

  1. 克隆仓库

  git clone https://github.com/xa1st/b2upload.git
  cd b2upload

  2. 编译项目（使用已编译的可执行文件或重新编译）

  # 重新编译（可选）
  go build -o b2upload.exe

  3. 配置认证信息 - 创建 b2upload.toml 配置文件

  # B2 认证信息（必须）
  token = "your_key_id:your_application_key"
  bucket = "your_bucket_name"

  # 默认下载域名
  base_url = "https://f000.backblazeb2.com/file"

  # 默认标签
  tag.default = "default"

  # 标签配置1
  [tag.default]
  username = "your_username"
  url = "https://your_domain.com"

  4. 开始上传文件

  # 上传单个文件
  ./b2upload.exe image.png

  # 上传目录下所有文件
  ./b2upload.exe ./images/*

  # 使用特定标签上传
  ./b2upload.exe --tag custom *.jpg

  ## ⌨️ 命令行参数说明

  | 参数 | 简写 | 类型 | 说明 |
  | --- | --- | --- | --- |
  | <文件或目录> | - | 路径 | 必选：要上传的文件、目录或通配符模式 |
  | --tag <标签名> | -t | 字符串 | 可选：指定使用的图床配置标签，覆盖默认标签 |
  | --help | -h | 开关 | 可选：显示完整的帮助信息 |
  | --version | -V | 开关 | 可选：显示当前版本号 |

  ## 📋 配置文件详解

  配置文件 b2upload.toml 支持以下字段：

  # 全局认证信息（必须）
  token = "key_id:application_key"    # B2 API认证令牌
  bucket = "bucket_name"              # B2存储桶名称
  base_url = "https://f000.backblazeb2.com/file"  # 默认下载域名

  # 标签管理
  tag.default = "default"             # 默认使用的标签名

  # 标签配置示例
  [tag.default]
  username = "your_username"          # B2用户名
  url = "https://your_domain.com"      # 自定义域名（可选，默认使用base_url）

  [tag.custom]
  username = "another_user"
  url = "https://another_domain.com"

  ## 🧩 技术栈揭秘

  | 模块功能 | 依赖库 | 作用说明 |
  | --- | --- | --- |
  | 命令行解析 | github.com/spf13/cobra | 提供强大的命令行参数解析和帮助文档生成 |
  | 配置文件管理 | github.com/spf13/viper | 支持TOML格式配置文件读取和环境变量管理 |
  | HTTP客户端 | Go标准库net/http | 处理与Backblaze B2 API的网络通信 |
  | 文件路径处理 | Go标准库path/filepath | 跨平台文件路径处理和通配符匹配 |
  | 并发控制 | Go标准库sync | 使用互斥锁和协程池实现安全的并发上传 |
  | MD5计算 | Go标准库crypto/md5 | 计算文件MD5值用于B2上传校验和文件命名 |

  ## 📄 工作流程

  1. 初始化配置 - 读取并合并命令行参数、配置文件、默认值
  2. B2授权 - 使用API密钥获取B2存储访问权限
  3. 文件扫描 - 根据输入模式查找待上传的文件列表
  4. 并发上传 - 使用工作协程池并发处理文件上传任务
  5. 结果汇总 - 统计上传成功率、耗时等信息并生成报告

  ## 💡 使用技巧

  1. 批量上传效率 - 使用通配符模式可以一次性上传多个同类型文件
  2. 域名管理 - 可以为不同用途配置不同的标签和域名
  3. 文件命名 - 远程路径格式为 [用户名]/[年份]/[月日]/[MD5前16位].[扩展名]
  4. 重复检测 - 工具会自动跳过已存在的文件，避免重复上传
  5. 错误处理 - 网络错误时程序会显示详细错误信息，便于排查问题

  ## 📄 许可证

  本项目基于 Apache2.0 License 开源，可自由用于个人/商业项目，详见 LICENSE (LICENSE) 文件。

  ## 💡 温馨提示

  1. 请确保B2 API密钥具有足够的权限（至少需要写入权限）
  2. 大文件上传可能需要较长时间，请耐心等待程序完成
  3. 如果遇到网络超时，可以检查网络连接或防火墙设置
  4. 建议定期备份重要的配置文件，避免丢失认证信息
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/xa1st/b2upload/internal/b2"
	"github.com/xa1st/b2upload/internal/config"
	"github.com/xa1st/b2upload/internal/util"
)

const version = "1.1.1.20251106" // 保持你的版本号

// rootCmd 是整个应用程序的根命令
var rootCmd = &cobra.Command{
	// 使用 <标签名> <文件名/文件夹> 作为参数顺序
	Use:     "b2upload <标签名> <文件名/文件夹>",
	Short:   "Backblaze B2 图床上传工具",
	Long:    `b2upload 是一个命令行工具，用于上传文件到 Backblaze B2 图床。标签名 (如 custom) 和至少一个文件/文件夹路径都必须提供。`,
	Version: version,
	Run:     runUpload,
}

// initConfig 是 Viper 初始化的关键函数，负责读取和合并配置
func initConfig() {
	// 1. 设置配置文件的搜索路径和名称
	viper.SetConfigName("b2upload") // 配置文件名应为 b2upload.toml
	viper.SetConfigType("toml")

	// 搜索路径 : 可执行文件所在的目录 (适用于编译后的 EXE)
	if ex, err := os.Executable(); err == nil {
		exePath := filepath.Dir(ex)
		viper.AddConfigPath(exePath)
	}

	// 2. 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			// 只有不是“文件未找到”的错误才打印警告
			fmt.Fprintf(os.Stderr, "警告: 读取配置文件失败: %v\n", err)
		}
	}
	// 3. 设置默认值 (仅设置全局项)
	// 确保这里使用 baseurl (而不是 base_url)，以匹配你的 TOML 文件
	viper.SetDefault("baseurl", "https://f000.backblazeb2.com/file")
}

func init() {
	// Cobra 支持多个根命令，但此处只有一个
	cobra.OnInitialize(initConfig)
	// 显示版本信息
	rootCmd.SetVersionTemplate("b2upload v{{.Version}}\n")
}

// runUpload 是实际执行文件上传逻辑的函数
func runUpload(cmd *cobra.Command, args []string) {
	// 检查参数数量：至少需要 2 个参数 (tag名 + 1个文件/文件夹)
	if len(args) < 2 { // 移除 && len(args) != 0，因为 len(args)=0 会被 Help() 捕获或不满足 len(args)<2
		if len(args) == 0 {
			cmd.Help() // 如果没有参数，显示帮助
			return
		}
		fmt.Fprintln(os.Stderr, "错误: 必须提供标签名和至少一个文件/文件夹路径。")
		cmd.Help()
		return
	}

	// 1. **标签解析和配置提取**
	tagName := args[0]       // 标签名现在是第一个位置参数
	filePatterns := args[1:] // 文件/文件夹路径是 args 剩余的部分

	// 获取开始时间
	startTime := time.Now()

	// 从配置文件根部获取 B2 认证和基础 URL
	token := viper.GetString("token")
	bucket := viper.GetString("bucket")
	baseUrl := viper.GetString("baseurl") // 读取 baseurl

	// 查找标签配置 (使用 [tags.XXX] 结构)
	tagKey := fmt.Sprintf("tags.%s", tagName) // 构造 Viper 路径：例如 "tags.mdd"
	user := viper.GetString(tagKey + ".username")
	tagUrl := viper.GetString(tagKey + ".url") // 读取标签下的 URL
	// 调试信息，在生产环境中可移除
	// fmt.Println(tagKey, user, tagUrl)

	// ***** 核心回退逻辑 (使用标签 URL 或 baseurl) *****
	finalUrl := tagUrl
	if finalUrl == "" {
		finalUrl = baseUrl // 回退到全局 baseurl
	}
	// ************************

	// 检查标签配置是否完整
	if user == "" {
		// 打印清晰的错误信息，引导用户检查 TOML 文件
		fmt.Fprintf(os.Stderr, "错误: 未能找到或解析配置标签 [%s]。请检查 b2upload.toml 文件中 [%s] 部分的 username 字段是否存在。\n", tagName, tagKey)
		os.Exit(1)
	}

	fmt.Printf("正在使用配置标签: [%s] (用户: %s, URL: %s)\n", tagName, user, finalUrl)

	// 2. 加载配置
	cfg, err := config.NewConfig(user, finalUrl, token, bucket)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// ----------------------------------------------------------------------------------
	// 3. 【优化】查找文件 (处理所有参数) - 提前到授权前
	var filesToUpload []string
	// 循环使用 filePatterns (即 args[1:])
	for _, pattern := range filePatterns {
		files, err := util.FindFiles(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "警告: 查找文件模式 %s 失败: %v\n", pattern, err)
			continue
		}
		filesToUpload = append(filesToUpload, files...)
	}

	if len(filesToUpload) == 0 {
		// 如果未找到文件，提前退出，避免 B2 授权 (网络连接)
		fmt.Println("未找到任何文件进行上传。")
		return
	}

	fmt.Printf("当前目录中共找到 %d 个文件，开始并发上传...\n", len(filesToUpload))
	// ----------------------------------------------------------------------------------

	// 4. 【网络操作】初始化上传器并进行 B2 授权 - 仅在确定有文件后执行
	uploader := b2.NewUploader(cfg)
	if err := uploader.AuthorizeAccount(); err != nil {
		fmt.Printf("B2 账户授权失败: %v\n", err)
		os.Exit(1)
	}

	// 5. 执行并发上传
	results := uploader.UploadFiles(filesToUpload)

	// 6. 打印结果和总结
	successCount := 0
	for _, res := range results {
		if res.Error != nil {
			fmt.Printf("上传失败，原文件是：%s，错误信息：%v\n", filepath.Base(res.LocalFile), res.Error)
		} else {
			fmt.Printf("上传成功，原文件是：%s 远程路径文件：%s\n", filepath.Base(res.LocalFile), res.PublicURL)
			successCount++
		}
	}

	duration := time.Since(startTime).Seconds()
	if successCount == len(filesToUpload) {
		fmt.Printf("全部 %d 个文件上传成功，本次用时 %.2f 秒\n", successCount, duration)
	} else {
		fmt.Printf("上传完成。成功 %d 个，失败 %d 个，本次用时 %.2f 秒\n", successCount, len(filesToUpload)-successCount, duration)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

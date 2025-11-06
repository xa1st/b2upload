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

const version = "1.0.0"

// cfgTag 用于存储命令行传入的 tag 名称，优先级最高
var cfgTag string

// rootCmd 是整个应用程序的根命令
var rootCmd = &cobra.Command{
	Use:     "b2upload <文件名/文件夹> --tag <标签名>",
	Short:   "Backblaze B2 图床上传工具",
	Long:    `b2upload 是一个命令行工具，用于上传文件到 Backblaze B2 图床。`,
	Version: version,
	Run:     runUpload,
}

// initConfig 是 Viper 初始化的关键函数，负责读取和合并配置
func initConfig() {
	// 1. 设置配置文件的搜索路径和名称
	viper.SetConfigName("b2upload") // 配置名称改为 b2upload.toml
	viper.SetConfigType("toml")
	// 设置搜索路径：当前目录
	viper.AddConfigPath(".")
	// 设置搜索路径：用户主目录
	if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(home)
	}
	// 2. 读取配置文件
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			fmt.Fprintf(os.Stderr, "-> 警告: 读取配置文件失败: %v\n", err)
		}
	}
	// 3. 设置默认值 (最低优先级)
	viper.SetDefault("tag.default", "private")
	// NEW: 设置默认 base_url
	viper.SetDefault("base_url", "https://f000.backblazeb2.com/file")
}

func init() {
	// Cobra 支持多个根命令，但此处只有一个
	cobra.OnInitialize(initConfig)
	// 定义命令行参数
	// 仅保留 TAG 参数用于配置切换
	rootCmd.PersistentFlags().StringVarP(&cfgTag, "tag", "t", "", "-> 指定使用的图床配置标签 (如 bsayde)，将覆盖 tag.default")
	// 显示版本信息
	rootCmd.SetVersionTemplate("-> b2upload 版本: {{.Version}}\n")
}

// runUpload 是实际执行文件上传逻辑的函数
func runUpload(cmd *cobra.Command, args []string) {
	if len(args) == 0 {
		cmd.Help()
		return
	}
	// 获取开始时间
	startTime := time.Now()
	// 1. **标签解析和配置提取**
	tagName := cfgTag
	if tagName == "" {
		tagName = viper.GetString("tag.default")
	}

	// 从配置文件根部获取 B2 认证和基础 URL
	token := viper.GetString("token")
	bucket := viper.GetString("bucket")
	baseUrl := viper.GetString("base_url") // NEW: 读取 base_url

	// 查找标签配置
	tagKey := fmt.Sprintf("tag.%s", tagName)
	user := viper.GetString(tagKey + ".username")
	tagUrl := viper.GetString(tagKey + ".url") // 读取标签下的 URL

	// ***** 核心回退逻辑 *****
	// 如果标签下没有指定 URL (tagUrl 为空)，则使用全局 base_url 作为 tag 的 URL
	// 这允许用户在 tag 下不设置 url 字段，直接使用 B2 默认的下载域名
	finalUrl := tagUrl
	if finalUrl == "" {
		finalUrl = baseUrl // 回退到全局 base_url
	}
	// ************************

	// 检查标签配置是否完整
	if user == "" {
		fmt.Fprintf(os.Stderr, "-> 错误: 未能找到或解析配置标签 [%s]。请检查 blogimg.toml 文件中 [%s] 部分的 username 字段。\n", tagName, tagKey)
		os.Exit(1)
	}

	fmt.Printf("-> 正在使用配置标签: [%s] (用户: %s, URL: %s)\n", tagName, user, finalUrl)

	// 2. 加载配置
	cfg, err := config.NewConfig(user, finalUrl, token, bucket)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	// 3. 初始化上传器并进行 B2 授权
	uploader := b2.NewUploader(cfg)
	if err := uploader.AuthorizeAccount(); err != nil {
		fmt.Printf("-> B2 账户授权失败: %v\n", err)
		os.Exit(1)
	}

	// 4. 查找文件 (处理所有参数)
	var filesToUpload []string
	for _, pattern := range args {
		files, err := util.FindFiles(pattern)
		if err != nil {
			fmt.Fprintf(os.Stderr, "-> 警告: 查找文件模式 %s 失败: %v\n", pattern, err)
			continue
		}
		filesToUpload = append(filesToUpload, files...)
	}

	if len(filesToUpload) == 0 {
		fmt.Println("-> 未找到任何文件进行上传。")
		return
	}

	fmt.Printf("-> 当前目录中共找到 %d 个文件，开始并发上传...\n", len(filesToUpload))

	// 5. 执行并发上传
	results := uploader.UploadFiles(filesToUpload)

	// 6. 打印结果和总结
	successCount := 0
	for _, res := range results {
		if res.Error != nil {
			fmt.Printf("-> 上传失败，原文件是：%s，错误信息：%v\n", filepath.Base(res.LocalFile), res.Error)
		} else {
			fmt.Printf("-> 上传成功，原文件是：%s 远程路径文件：%s\n", filepath.Base(res.LocalFile), res.PublicURL)
			successCount++
		}
	}

	duration := time.Since(startTime).Seconds()
	if successCount == len(filesToUpload) {
		fmt.Printf("-> 全部 %d 个文件上传成功，本次用时 %.2f 秒\n", successCount, duration)
	} else {
		fmt.Printf("-> 上传完成。成功 %d 个，失败 %d 个，本次用时 %.2f 秒\n", successCount, len(filesToUpload)-successCount, duration)
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

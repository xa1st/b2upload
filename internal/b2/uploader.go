package b2

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/xa1st/b2upload/internal/config"
	"github.com/xa1st/b2upload/internal/util"
)

// --- B2 API 响应结构体 ---

// StorageAPIInfo 用于捕获 JSON 中 "storageApi" 的核心信息
type StorageAPIInfo struct {
	APIURL      string `json:"apiUrl"`
	DownloadURL string `json:"downloadUrl"`
	BucketID    string `json:"bucketId"`   // 捕获 Bucket ID
	BucketName  string `json:"bucketName"` // 捕获 Bucket Name
}

// APIInfo 捕获 b2_authorize_account 响应中的 API 信息组
type APIInfo struct {
	// 匹配您响应中的 "storageApi" 字段
	StorageAPI StorageAPIInfo `json:"storageApi"`
	// 兼容标准的 V4 结构中的 "b2" 字段
	B2 StorageAPIInfo `json:"b2"`
}

// AuthResponse 是 b2_authorize_account 的响应
type AuthResponse struct {
	AuthorizationToken string `json:"authorizationToken"`

	APIInfo APIInfo `json:"apiInfo"`

	// 根级字段 (兼容旧版本或通用字段)
	APIURL      string `json:"apiUrl"`
	DownloadURL string `json:"downloadUrl"`

	AccountID string `json:"accountId"`

	// NEW: 存储最终提取的 Bucket ID
	BucketIDToUse string // Populated by AuthorizeAccount
}

// UploadURLResponse b2_get_upload_url 的响应
type UploadURLResponse struct {
	UploadURL                string `json:"uploadUrl"`          // 文件上传专用的 URL
	UploadAuthorizationToken string `json:"authorizationToken"` // 文件上传专用的 Token
}

// UploadFileResponse b2_upload_file 的响应
type UploadFileResponse struct {
	FileName string `json:"fileName"`
}

// ListFileNamesResponse b2_list_file_names 的响应
type ListFileNamesResponse struct {
	Files        []UploadFileResponse `json:"files"`
	NextFileName string               `json:"nextFileName"`
}

// UploadResult 存储单个文件上传的结果
type UploadResult struct {
	LocalFile string
	PublicURL string
	Error     error
	Skipped   bool // 新增字段，标记是否因已存在而跳过
}

// Uploader 包含 B2 上传所需的配置和授权信息
type Uploader struct {
	Config   *config.Config
	Auth     *AuthResponse
	Client   *http.Client
	UploadMu sync.Mutex // 保护上传 URL 资源的互斥锁
}

const (
	authorizeURL     = "https://api.backblazeb2.com/b2api/v3/b2_authorize_account"
	concurrencyLimit = 5
)

// NewUploader 创建一个新的 Uploader 实例
func NewUploader(cfg *config.Config) *Uploader {
	return &Uploader{
		Config: cfg,
		Client: &http.Client{Timeout: 60 * time.Second}, // 延长超时时间以适应大文件
	}
}

// AuthorizeAccount 执行 B2 授权流程 (保持不变)
func (u *Uploader) AuthorizeAccount() error {
	fmt.Println("-> 正在进行 B2 授权...")
	// B2 认证需要 Basic Auth，将 keyId:key 进行 Base64 编码
	authString := base64.StdEncoding.EncodeToString([]byte(u.Config.Token))

	req, err := http.NewRequest("GET", authorizeURL, nil)
	if err != nil {
		return fmt.Errorf("-> 创建授权请求失败: %w", err)
	}

	req.Header.Add("Authorization", "Basic "+authString)

	resp, err := u.Client.Do(req)
	if err != nil {
		return fmt.Errorf("-> B2 授权网络请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("-> B2 授权失败 (状态码: %d), 响应: %s", resp.StatusCode, string(body))
	}

	var auth AuthResponse
	// 读取完整的 Body
	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, &auth); err != nil {
		return fmt.Errorf("-> 解析 B2 授权响应失败: %w", err)
	}

	// ****** 关键修复逻辑：提取 URL 和 Bucket ID ******

	// 1. 提取 APIURL 和 DownloadURL，优先使用您响应中的 storageApi 结构
	if auth.APIURL == "" && auth.APIInfo.StorageAPI.APIURL != "" {
		auth.APIURL = auth.APIInfo.StorageAPI.APIURL
	}
	if auth.DownloadURL == "" && auth.APIInfo.StorageAPI.DownloadURL != "" {
		auth.DownloadURL = auth.APIInfo.StorageAPI.DownloadURL
	}
	// 2. 提取 Bucket ID，也优先使用 storageApi 结构
	if auth.APIInfo.StorageAPI.BucketID != "" {
		auth.BucketIDToUse = auth.APIInfo.StorageAPI.BucketID
	}

	// Fallback: 兼容标准的 V4 结构
	if auth.APIURL == "" && auth.APIInfo.B2.APIURL != "" {
		auth.APIURL = auth.APIInfo.B2.APIURL
	}
	if auth.DownloadURL == "" && auth.APIInfo.B2.DownloadURL != "" {
		auth.DownloadURL = auth.APIInfo.B2.DownloadURL
	}
	if auth.BucketIDToUse == "" && auth.APIInfo.B2.BucketID != "" {
		auth.BucketIDToUse = auth.APIInfo.B2.BucketID
	}
	// ********************************************

	if auth.APIURL == "" {
		return fmt.Errorf("-> B2 授权响应结构异常：未找到 apiUrl 字段。请检查您的 AppKey 权限。原始响应: %s", string(bodyBytes))
	}

	if auth.BucketIDToUse == "" {
		return fmt.Errorf("-> B2 授权成功，但未在响应中找到 Bucket ID。请确认您的 App Key 是针对单个 Bucket 创建的，或提供更高级别的 App Key。")
	}

	u.Auth = &auth

	fmt.Println("-> B2 API URL 解析成功")
	fmt.Println("-> B2 Bucket ID 解析成功")
	return nil
}

// getUploadURL 获取文件上传专用的 URL 和 Token (保持不变)
func (u *Uploader) getUploadURL() (*UploadURLResponse, error) {
	if u.Auth == nil {
		return nil, fmt.Errorf("-> 尚未授权 B2 账户")
	}

	bucketID := u.Auth.BucketIDToUse
	if bucketID == "" {
		return nil, fmt.Errorf("-> Bucket ID 缺失，无法获取上传 URL。请重新授权。")
	}

	// 构造 b2_get_upload_url 请求体
	requestBody, _ := json.Marshal(map[string]string{
		"bucketId": bucketID,
	})

	// 使用解析成功的 APIURL
	url := u.Auth.APIURL + "/b2api/v3/b2_get_upload_url"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("-> 创建获取上传URL请求失败: %w", err)
	}

	req.Header.Set("Authorization", u.Auth.AuthorizationToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("-> 获取上传URL网络请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("-> 获取上传URL失败 (状态码: %d), 响应: %s", resp.StatusCode, string(body))
	}

	var uploadResp UploadURLResponse
	if err := json.NewDecoder(resp.Body).Decode(&uploadResp); err != nil {
		return nil, fmt.Errorf("-> 解析上传URL响应失败: %w", err)
	}

	return &uploadResp, nil
}

// buildPublicURL 构造最终的公开 URL
func (u *Uploader) buildPublicURL(remotePath string) string {
	if u.Config.URL != "" {
		// 使用自定义域名 (URL)
		// 移除配置用户前缀，以符合用户定义的URL结构
		userPrefix := u.Config.User + "/"
		cleanRemotePath := strings.TrimPrefix(remotePath, userPrefix)

		// 确保 URL 带有斜杠
		finalURL := u.Config.URL
		if !strings.HasSuffix(finalURL, "/") {
			finalURL += "/"
		}
		return finalURL + cleanRemotePath
	}

	// 使用 B2 官方下载域名 (Auth.DownloadURL)
	return fmt.Sprintf("%s/file/%s/%s", u.Auth.DownloadURL, u.Config.Bucket, remotePath)
}

// checkFileExists 检查文件是否已存在于 B2 存储桶中
func (u *Uploader) checkFileExists(remotePath string) (string, bool, error) {
	if u.Auth == nil || u.Auth.BucketIDToUse == "" {
		return "", false, fmt.Errorf("-> 授权信息不完整，无法检查文件存在性")
	}

	// 构造 b2_list_file_names 请求体：只请求一个文件
	requestBody, _ := json.Marshal(map[string]interface{}{
		"bucketId":      u.Auth.BucketIDToUse,
		"startFileName": remotePath, // 从该文件名开始查找
		"maxFileCount":  1,          // 只查找一个
	})

	url := u.Auth.APIURL + "/b2api/v3/b2_list_file_names"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return "", false, fmt.Errorf("-> 创建文件列表请求失败: %w", err)
	}

	req.Header.Set("Authorization", u.Auth.AuthorizationToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := u.Client.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("-> 文件列表网络请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", false, fmt.Errorf("-> 文件列表请求失败 (状态码: %d), 响应: %s", resp.StatusCode, string(body))
	}

	var listResp ListFileNamesResponse
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		return "", false, fmt.Errorf("-> 解析文件列表响应失败: %w", err)
	}

	// 检查返回的文件列表：如果文件列表不为空，且第一个文件的名字就是我们要找的，则文件存在。
	if len(listResp.Files) > 0 && listResp.Files[0].FileName == remotePath {
		publicURL := u.buildPublicURL(remotePath)
		return publicURL, true, nil
	}

	return "", false, nil
}

// uploadSingleFile 执行单个文件的 B2 上传操作
func (u *Uploader) uploadSingleFile(localFilePath, remotePath string, uploadInfo *UploadURLResponse) (string, error) {
	// *** 1. 检查文件是否存在 ***
	publicURL, exists, err := u.checkFileExists(remotePath)
	if err != nil {
		// 如果检查失败，我们选择继续尝试上传，但记录警告
		fmt.Printf("-> 警告：检查文件存在性失败 (%s)，将尝试上传: %v\n", remotePath, err)
	}
	if exists {
		return publicURL, nil // 文件已存在，直接返回 URL，跳过后续上传流程
	}

	// 2. 准备文件数据和元数据
	file, err := os.Open(localFilePath)
	if err != nil {
		return "", fmt.Errorf("-> 无法打开本地文件 %s: %w", localFilePath, err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("-> 无法获取文件信息: %w", err)
	}
	// 计算 MD5
	fileMD5, err := util.CalculateFileMD5(localFilePath)
	if err != nil {
		return "", err
	}
	// 猜测 Content Type
	contentType := mime.TypeByExtension(strings.ToLower(filepath.Ext(localFilePath)))
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// 3. 构造 b2_upload_file 请求
	req, err := http.NewRequest("POST", uploadInfo.UploadURL, file)
	if err != nil {
		return "", fmt.Errorf("-> 创建上传请求失败: %w", err)
	}

	// 必须的请求头
	req.Header.Set("Authorization", uploadInfo.UploadAuthorizationToken)
	req.Header.Set("X-Bz-File-Name", remotePath)
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = fileInfo.Size()

	// 校验头: 使用 Content-MD5 (B2 兼容)
	req.Header.Set("X-Bz-Content-Md5", fileMD5)
	// B2 官方推荐 X-Bz-Content-Sha1，这里使用 "do_not_verify" 以避免计算 SHA-1
	req.Header.Set("X-Bz-Content-Sha1", "do_not_verify")

	// 4. 执行上传
	resp, err := u.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("-> B2 上传网络请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 5. 处理响应
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("-> B2 上传失败 (状态码: %d), 响应: %s", resp.StatusCode, string(body))
	}

	// 6. 构造最终的公开 URL
	return u.buildPublicURL(remotePath), nil
}

// UploadFiles 并发上传文件列表
func (u *Uploader) UploadFiles(filesToUpload []string) []UploadResult {
	results := make(chan UploadResult, len(filesToUpload))
	paths := make(chan string, len(filesToUpload))
	var wg sync.WaitGroup

	// 在主协程中获取上传 URL
	uploadInfo, err := u.getUploadURL()

	// 启动工作协程
	numWorkers := concurrencyLimit
	if len(filesToUpload) < concurrencyLimit {
		numWorkers = len(filesToUpload)
	}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 每个工作协程从 paths 队列中取出文件并上传
			for localFile := range paths {
				cleanLocalFile := filepath.Clean(localFile)
				result := UploadResult{LocalFile: cleanLocalFile}

				// 如果获取 uploadInfo 失败，则直接记录错误
				if err != nil {
					result.Error = fmt.Errorf("-> 无法上传，B2 上传 URL 获取失败: %w", err)
					results <- result
					continue
				}
				// 1. 生成远程路径
				remotePath, pathErr := util.GenerateRemotePath(cleanLocalFile, u.Config.User)
				if pathErr != nil {
					result.Error = fmt.Errorf("-> 无法生成远程路径: %w", pathErr)
					results <- result
					continue
				}

				// 打印上传文件名称和远程路径信息
				fmt.Printf("-> 准备处理 %s 到 B2 路径: %s\n", filepath.Base(cleanLocalFile), remotePath)

				// 2. 执行上传
				publicURL, uploadErr := u.uploadSingleFile(cleanLocalFile, remotePath, uploadInfo)
				if uploadErr != nil {
					result.Error = uploadErr
				} else {
					result.PublicURL = publicURL
					// 检查是否跳过：如果返回的 publicURL 是来自已存在文件，我们认为它被跳过了。
					// 这里可以根据业务逻辑调整，但通常如果上传成功，exist是false，如果跳过，exist是true。
					_, exists, _ := u.checkFileExists(remotePath)
					if exists && uploadErr == nil {
						result.Skipped = true
					}
				}
				results <- result
			}
		}()
	}
	// 路径发送和通道关闭
	for _, f := range filesToUpload {
		paths <- f
	}
	close(paths)
	// 等待所有工作协程完成
	wg.Wait()
	close(results)
	return u.collectResults(results, len(filesToUpload))
}

// collectResults 从结果通道中收集所有结果
func (u *Uploader) collectResults(results chan UploadResult, count int) []UploadResult {
	finalResults := make([]UploadResult, 0, count)
	for res := range results {
		finalResults = append(finalResults, res)
	}
	return finalResults
}

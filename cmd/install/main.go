package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"strings"

	"github.com/AlecAivazis/survey/v2"
	"github.com/fatih/color"
	"github.com/joho/godotenv"
)

const (
	exampleFile        = "example.env"
	targetFile         = ".env"
	defaultComposeFile = "docker-compose.yaml"
	sslComposeFile     = "docker-compose-ssl.yaml"
	caddyfile          = "data/Caddyfile"
	exampleCaddyfile   = "data/example.Caddyfile"
)

// Config 保存所有配置
type Config struct {
	Language           string
	Domain             string
	Email              string
	UseSSL             bool
	Port               string
	MySQLRootPassword  string
	MySQLDatabase      string
	MySQLUser          string
	MySQLPassword      string
	AdminLoginUser     string
	AdminLoginPassword string
	envVars            map[string]string
}

var (
	green  = color.New(color.FgGreen)
	red    = color.New(color.FgRed)
	yellow = color.New(color.FgYellow)
	cyan   = color.New(color.FgCyan)
	bold   = color.New(color.Bold)
)

func main() {
	bold.Println("netiCRM Self-Host 自架站台安裝程式")
	fmt.Println()

	// 檢查階段
	if err := goCheck(); err != nil {
		red.Printf("✗ 檢查失敗: %v\n", err)
		os.Exit(1)
	}

	// 詢問階段
	cfg, err := goAsk()
	if err != nil {
		red.Printf("✗ 設定失敗: %v\n", err)
		os.Exit(1)
	}

	// 執行階段
	if err := goRun(cfg); err != nil {
		red.Printf("✗ 執行失敗: %v\n", err)
		os.Exit(1)
	}

	green.Println("✅ 安裝完成！")
}

// goCheck 進行所有事前檢查
func goCheck() error {
	// 檢查是否有 .env 和資料庫檔案
	hasEnv := fileExists(targetFile)
	hasMariaDBData := checkMariaDBData()

	if hasEnv && hasMariaDBData {
		yellow.Println("發現現有的資料庫檔案，看起來這是一個已經安裝好的網站。")

		// 讀取現有配置
		existingEnv, _ := godotenv.Read(targetFile)
		domain := existingEnv["DOMAIN"]
		port := existingEnv["HTTP_PORT"]
		adminUser := existingEnv["ADMIN_LOGIN_USER"]

		// 如果有 Caddyfile，嘗試從中獲取域名
		if fileExists(caddyfile) {
			if caddyDomain := getDomainFromCaddyfile(); caddyDomain != "" {
				domain = caddyDomain
			}
		}

		fmt.Println()
		cyan.Println("現有配置：")
		if domain != "" && domain != "localhost" {
			fmt.Printf("  域名 Domain: %s\n", domain)
		}
		if port != "" {
			fmt.Printf("  端口 Port: %s\n", port)
		}
		if adminUser != "" {
			fmt.Printf("  管理員帳號: %s\n", adminUser)
		}
		fmt.Println()

		options := []string{
			"1. 執行 docker 啟動指令（若已啟動則不影響）",
			"2. 備份網站檔案並覆蓋設定",
			"3. 檢視初始設定管理員密碼 ADMIN_LOGIN_PASSWORD",
			"4. 結束安裝",
		}

		var choice string
		prompt := &survey.Select{
			Message: "請選擇操作（上下鍵選取，或按下數字鍵後 enter）：",
			Options: options,
		}
		if err := survey.AskOne(prompt, &choice); err != nil {
			return err
		}

		switch choice {
		case options[0]: // 執行 docker 啟動指令
			return startDocker()
		case options[1]: // 備份並覆蓋配置
			if err := backupExisting(); err != nil {
				return err
			}
		case options[2]: // 檢視密碼
			yellow.Println("⚠️  注意：此會用明文顯示初始密碼，且可能已更改")
			var confirmShow bool
			confirmPrompt := &survey.Confirm{
				Message: "確定要顯示密碼嗎？",
				Default: false,
			}
			if err := survey.AskOne(confirmPrompt, &confirmShow); err != nil {
				return err
			}

			if confirmShow {
				if pass := existingEnv["ADMIN_LOGIN_PASSWORD"]; pass != "" {
					fmt.Printf("ADMIN_LOGIN_PASSWORD: %s\n", pass)
				} else {
					fmt.Println("密碼未設定或為空")
				}
			}
			os.Exit(0)
		case options[3]: // 結束安裝
			fmt.Println("安裝取消。")
			os.Exit(0)
		}
	} else if hasEnv {
		// 只有 .env 沒有資料庫
		yellow.Println("發現現有的 .env 檔案")

		// 讀取並顯示現有配置
		existingEnv, _ := godotenv.Read(targetFile)
		domain := existingEnv["DOMAIN"]
		port := existingEnv["HTTP_PORT"]
		adminUser := existingEnv["ADMIN_LOGIN_USER"]

		// 如果有 Caddyfile，優先使用其中的域名
		if fileExists(caddyfile) {
			if caddyDomain := getDomainFromCaddyfile(); caddyDomain != "" {
				domain = caddyDomain
			}
		}

		if domain != "" || port != "" || adminUser != "" {
			fmt.Println()
			cyan.Println("現有配置：")
			if domain != "" && domain != "localhost" {
				fmt.Printf("  域名: %s\n", domain)
			}
			if port != "" {
				fmt.Printf("  端口: %s\n", port)
			}
			if adminUser != "" {
				fmt.Printf("  管理員: %s\n", adminUser)
			}
			fmt.Println()
		}

		var overwrite bool
		prompt := &survey.Confirm{
			Message: "是否要更改設定？(舊的 .env 檔會改名備份)",
			Default: false,
		}
		if err := survey.AskOne(prompt, &overwrite); err != nil {
			return err
		}

		if !overwrite {
			fmt.Println("安裝取消。")
			os.Exit(0)
		}

		if err := backupFile(targetFile); err != nil {
			return err
		}
	}

	// 檢查 Docker
	if err := checkDocker(); err != nil {
		yellow.Printf("⚠️  %v\n", err)

		var proceed bool
		prompt := &survey.Confirm{
			Message: "是否要繼續僅更改 .env 檔案？",
			Default: false,
		}
		if err := survey.AskOne(prompt, &proceed); err != nil {
			return err
		}

		if !proceed {
			fmt.Println("建議先安裝 Docker，安裝取消。")
			os.Exit(0)
		}
	}

	// 檢查 Caddyfile
	if fileExists(caddyfile) {
		cyan.Println("發現 Caddyfile，可使用 SSL 配置。")
		if domain := getDomainFromCaddyfile(); domain != "" {
			fmt.Printf("現有 SSL 域名：%s\n", domain)
		}
	}

	return nil
}

// goAsk 進行所有互動詢問
func goAsk() (*Config, error) {
	cfg := &Config{
		envVars: make(map[string]string),
	}

	// 載入預設環境變數
	if err := loadDefaultEnvs(cfg); err != nil {
		return nil, err
	}

	// 1. 語言選擇
	if err := askLanguage(cfg); err != nil {
		return nil, err
	}

	// 2. 域名和 SSL 設定
	fmt.Println()
	if cfg.Language == "zh-hant" {
		cyan.Println("若您已有域名（Domain），請先將域名以A紀錄設到本主機 IP")
		cyan.Println("本安裝程式可自動幫您設定 SSL 並綁定網域")
		cyan.Println("或依照您所選的設定綁定特定埠（Port）")
	} else {
		cyan.Println("If you already have a domain name, please point it to this host's IP address.")
		cyan.Println("This installer can automatically set up SSL and bind the domain for you,")
		cyan.Println("or bind to a specific port according to your chosen settings.")
	}

	if err := askDomainAndSSL(cfg); err != nil {
		return nil, err
	}

	// 3. MySQL 設定
	if err := askMySQL(cfg); err != nil {
		return nil, err
	}

	// 4. 管理員帳號密碼
	if err := askAdminCredentials(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// goRun 執行寫入和啟動
func goRun(cfg *Config) error {
	// 設定環境變數
	cfg.envVars["LANGUAGE"] = cfg.Language

	if !cfg.UseSSL {
		if cfg.Domain != "" {
			cfg.envVars["DOMAIN"] = cfg.Domain
			cfg.envVars["HTTP_PORT"] = ""
		} else {
			cfg.envVars["DOMAIN"] = "localhost"
			cfg.envVars["HTTP_PORT"] = cfg.Port
		}
	} else {
		cfg.envVars["HTTP_PORT"] = ""
	}

	// MySQL 設定
	cfg.envVars["MYSQL_ROOT_PASSWORD"] = cfg.MySQLRootPassword
	if cfg.MySQLDatabase != "" {
		cfg.envVars["MYSQL_DATABASE"] = cfg.MySQLDatabase
	}
	if cfg.MySQLUser != "" {
		cfg.envVars["MYSQL_USER"] = cfg.MySQLUser
	}
	cfg.envVars["MYSQL_PASSWORD"] = cfg.MySQLPassword

	// 管理員設定
	cfg.envVars["ADMIN_LOGIN_USER"] = cfg.AdminLoginUser
	cfg.envVars["ADMIN_LOGIN_PASSWORD"] = cfg.AdminLoginPassword

	// 寫入 .env
	if err := writeEnvFile(cfg); err != nil {
		return fmt.Errorf("寫入 .env 失敗: %w", err)
	}

	// 更新 Caddyfile
	if cfg.UseSSL {
		if err := updateCaddyfile(cfg); err != nil {
			return fmt.Errorf("更新 Caddyfile 失敗: %w", err)
		}
	}

	// 選擇 compose 檔案
	composeFile := defaultComposeFile
	if cfg.UseSSL {
		composeFile = sslComposeFile
	}

	green.Printf("✅ .env 建立完成\n")

	// 檢查是否有 Docker
	if err := checkDocker(); err != nil {
		yellow.Println("Docker Compose 未安裝，請手動執行：")
		fmt.Printf("docker compose -f %s up -d\n", composeFile)
		return nil
	}

	// 執行 docker compose
	fmt.Printf("開始執行 docker compose -f %s up -d ...\n", composeFile)
	if err := dockerComposeUp(composeFile); err != nil {
		return err
	}

	cyan.Println("服務已啟動，可使用以下指令查看日誌：")
	fmt.Printf("docker compose -f %s logs -f\n", composeFile)

	return nil
}

// 輔助函數

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func checkMariaDBData() bool {
	mariadbPath := "data/mariadb_data"
	info, err := os.Stat(mariadbPath)
	if os.IsNotExist(err) || !info.IsDir() {
		return false
	}

	files, err := os.ReadDir(mariadbPath)
	return err == nil && len(files) > 0
}

func getDomainFromCaddyfile() string {
	data, err := os.ReadFile(caddyfile)
	if err != nil {
		return ""
	}

	// 使用正則表達式尋找域名
	// 支援多種格式：
	// - example.com {
	// - https://example.com {
	// - example.com:443 {
	// - example.com, www.example.com {
	content := string(data)
	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 跳過註解和空行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 尋找包含 { 的行
		if strings.Contains(line, "{") {
			// 提取 { 之前的部分
			parts := strings.Split(line, "{")
			if len(parts) > 0 {
				domainPart := strings.TrimSpace(parts[0])
				// 移除協議前綴
				domainPart = strings.TrimPrefix(domainPart, "https://")
				domainPart = strings.TrimPrefix(domainPart, "http://")
				// 移除端口
				if idx := strings.Index(domainPart, ":"); idx != -1 {
					domainPart = domainPart[:idx]
				}
				// 如果有多個域名（逗號分隔），取第一個
				if strings.Contains(domainPart, ",") {
					domains := strings.Split(domainPart, ",")
					domainPart = strings.TrimSpace(domains[0])
				}
				// 驗證是否為有效域名
				if domainPart != "" && strings.Contains(domainPart, ".") {
					return domainPart
				}
			}
		}
	}

	return ""
}

func backupFile(path string) error {
	backupPath := path + ".bak"
	count := 0

	for fileExists(backupPath) {
		count++
		backupPath = fmt.Sprintf("%s.bak%d", path, count)
	}

	if err := os.Rename(path, backupPath); err != nil {
		return fmt.Errorf("無法備份 %s: %v", path, err)
	}

	green.Printf("已將 %s 備份為 %s\n", path, backupPath)
	return nil
}

func backupExisting() error {
	// 備份 .env
	if err := backupFile(targetFile); err != nil {
		return err
	}

	// 詢問是否備份資料庫
	if checkMariaDBData() {
		var backupDB bool
		prompt := &survey.Confirm{
			Message: "是否要備份資料庫、網站檔案（data/mariadb_data、data/www 資料夾）？",
			Default: true,
		}
		if err := survey.AskOne(prompt, &backupDB); err != nil {
			return err
		}

		if backupDB {
			if err := backupFile("data/mariadb_data"); err != nil {
				return err
			}

			// 同時備份 data/www
			if fileExists("data/www") {
				if err := backupFile("data/www"); err != nil {
					yellow.Printf("警告: 無法備份 data/www: %v\n", err)
				}
			}
		}
	}

	return nil
}

func startDocker() error {
	hasCaddyfile := fileExists(caddyfile)

	var composeFile string
	if hasCaddyfile {
		cyan.Println("使用 SSL 配置啟動...")
		composeFile = sslComposeFile
	} else {
		cyan.Println("使用非 SSL 配置啟動...")
		composeFile = defaultComposeFile
	}

	return dockerComposeUp(composeFile)
}

func checkDocker() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("Docker 未安裝")
	}

	if err := exec.Command("docker", "compose", "version").Run(); err != nil {
		return fmt.Errorf("Docker Compose 插件未安裝或未啟用")
	}

	return nil
}

func dockerComposeUp(composeFile string) error {
	cmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("執行 docker compose up 失敗: %w", err)
	}

	green.Println("網站已成功啟動！")
	os.Exit(0)
	return nil
}

func loadDefaultEnvs(cfg *Config) error {
	data, err := os.ReadFile(exampleFile)
	if err != nil {
		return fmt.Errorf("讀取 %s 失敗: %w", exampleFile, err)
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}

		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			cfg.envVars[key] = val
		}
	}

	return nil
}

func askLanguage(cfg *Config) error {
	prompt := &survey.Select{
		Message: "What's your language? / 請選擇語言（上下鍵選取，或按下數字鍵後 enter）：",
		Options: []string{"1. English", "2. Taiwan Traditional Chinese 台灣繁體中文"},
	}

	var choice string
	if err := survey.AskOne(prompt, &choice); err != nil {
		return err
	}

	if choice == "1. English" {
		cfg.Language = "en"
	} else {
		cfg.Language = "zh-hant"
	}

	return nil
}

func askDomainAndSSL(cfg *Config) error {
	// SSL 詢問
	sslPrompt := "Do you have a domain and want to set up SSL automatically?"
	if cfg.Language == "zh-hant" {
		sslPrompt = "您是否有網域並希望自動設定 SSL？"
	}

	var useSSL bool
	prompt := &survey.Confirm{
		Message: sslPrompt,
		Default: false,
	}
	if err := survey.AskOne(prompt, &useSSL); err != nil {
		return err
	}
	cfg.UseSSL = useSSL

	if useSSL {
		// SSL 路徑
		domainPrompt := "Please enter your domain name (e.g., example.com):"
		emailPrompt := "Please enter your email (for Let's Encrypt SSL certificate):"
		if cfg.Language == "zh-hant" {
			domainPrompt = "請輸入您的域名 (例如 example.com)："
			emailPrompt = "請輸入您的電子郵件 (用於 Let's Encrypt SSL 證書)："
		}

		// 域名
		domainInput := &survey.Input{
			Message: domainPrompt,
		}
		if err := survey.AskOne(domainInput, &cfg.Domain, survey.WithValidator(survey.Required)); err != nil {
			return err
		}

		// Email
		emailInput := &survey.Input{
			Message: emailPrompt,
		}
		if err := survey.AskOne(emailInput, &cfg.Email); err != nil {
			return err
		}
	} else {
		// 非 SSL 路徑
		domainPrompt := "Domain (leave blank for no domain):"
		if cfg.Language == "zh-hant" {
			domainPrompt = "網站網址 (domain，可留空)："
		}

		domainInput := &survey.Input{
			Message: domainPrompt,
		}
		if err := survey.AskOne(domainInput, &cfg.Domain); err != nil {
			return err
		}

		if cfg.Domain == "" {
			portPrompt := "Please enter Port (default 8080):"
			if cfg.Language == "zh-hant" {
				portPrompt = "請輸入 Port (預設 8080)："
			}

			portInput := &survey.Input{
				Message: portPrompt,
				Default: "8080",
			}
			if err := survey.AskOne(portInput, &cfg.Port); err != nil {
				return err
			}
		}
	}

	return nil
}

func askMySQL(cfg *Config) error {
	modifyPrompt := "Modify MySQL parameters?"
	if cfg.Language == "zh-hant" {
		modifyPrompt = "是否要修改 MySQL 參數？"
	}

	var modify bool
	prompt := &survey.Confirm{
		Message: modifyPrompt,
		Default: false,
	}
	if err := survey.AskOne(prompt, &modify); err != nil {
		return err
	}

	if !modify {
		// 自動產生密碼
		cfg.MySQLRootPassword = randomPass(13)
		cfg.MySQLPassword = randomPass(13)
		// Database 和 User 保留預設值
		return nil
	}

	// ROOT 密碼
	if err := askPasswordWithConfirm(cfg, "MYSQL_ROOT_PASSWORD", &cfg.MySQLRootPassword, 13); err != nil {
		return err
	}

	// Database
	dbPrompt := "MYSQL_DATABASE (leave blank for default):"
	if cfg.Language == "zh-hant" {
		dbPrompt = "MYSQL_DATABASE (留空使用預設值)："
	}
	dbInput := &survey.Input{
		Message: dbPrompt,
	}
	if err := survey.AskOne(dbInput, &cfg.MySQLDatabase); err != nil {
		return err
	}

	// User
	userPrompt := "MYSQL_USER (leave blank for default):"
	if cfg.Language == "zh-hant" {
		userPrompt = "MYSQL_USER (留空使用預設值)："
	}
	userInput := &survey.Input{
		Message: userPrompt,
	}
	if err := survey.AskOne(userInput, &cfg.MySQLUser); err != nil {
		return err
	}

	// User 密碼
	if err := askPasswordWithConfirm(cfg, "MYSQL_PASSWORD", &cfg.MySQLPassword, 13); err != nil {
		return err
	}

	return nil
}

func askAdminCredentials(cfg *Config) error {
	// Username
	userPrompt := "ADMIN_LOGIN_USER (leave blank for 'admin'):"
	if cfg.Language == "zh-hant" {
		userPrompt = "ADMIN_LOGIN_USER (留空使用 'admin')："
	}

	userInput := &survey.Input{
		Message: userPrompt,
		Default: "admin",
	}
	if err := survey.AskOne(userInput, &cfg.AdminLoginUser); err != nil {
		return err
	}

	// Password
	if err := askPasswordWithConfirm(cfg, "ADMIN_LOGIN_PASSWORD", &cfg.AdminLoginPassword, 11); err != nil {
		return err
	}

	return nil
}

func askPasswordWithConfirm(cfg *Config, field string, target *string, defaultLen int) error {
	passPrompt := fmt.Sprintf("%s (leave blank for random password):", field)
	confirmPrompt := fmt.Sprintf("Please re-enter %s to confirm:", field)
	mismatchMsg := "✗ Passwords do not match. Please re-enter."

	if cfg.Language == "zh-hant" {
		passPrompt = fmt.Sprintf("%s (留空自動產生)：", field)
		confirmPrompt = fmt.Sprintf("請再次輸入%s密碼確認：", field)
		mismatchMsg = "✗ 兩次密碼不一致，請重新輸入。"
	}

	for {
		var password string
		passwordInput := &survey.Password{
			Message: passPrompt,
		}
		if err := survey.AskOne(passwordInput, &password); err != nil {
			return err
		}

		if password == "" {
			*target = randomPass(defaultLen)
			return nil
		}

		var confirm string
		confirmInput := &survey.Password{
			Message: confirmPrompt,
		}
		if err := survey.AskOne(confirmInput, &confirm); err != nil {
			return err
		}

		if password == confirm {
			*target = password
			return nil
		}

		red.Println(mismatchMsg)
	}
}

func randomPass(length int) string {
	// 定義字符集
	const (
		lowercase = "abcdefghijklmnopqrstuvwxyz"
		uppercase = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		digits    = "0123456789"
		symbols   = "!@#$%^&*"
	)
	allChars := lowercase + uppercase + digits + symbols

	// 確保密碼包含各種字符
	var password strings.Builder

	// 至少包含一個小寫字母
	password.WriteByte(lowercase[randInt(len(lowercase))])
	// 至少包含一個大寫字母
	password.WriteByte(uppercase[randInt(len(uppercase))])
	// 至少包含一個數字
	password.WriteByte(digits[randInt(len(digits))])
	// 至少包含一個符號
	password.WriteByte(symbols[randInt(len(symbols))])

	// 填充剩餘長度
	for i := 4; i < length; i++ {
		password.WriteByte(allChars[randInt(len(allChars))])
	}

	// 打亂密碼順序
	runes := []rune(password.String())
	for i := len(runes) - 1; i > 0; i-- {
		j := randInt(i + 1)
		runes[i], runes[j] = runes[j], runes[i]
	}

	return string(runes)
}

func randInt(max int) int {
	n, _ := rand.Int(rand.Reader, big.NewInt(int64(max)))
	return int(n.Int64())
}

func writeEnvFile(cfg *Config) error {
	data, err := os.ReadFile(exampleFile)
	if err != nil {
		// 如果無法讀取範例檔，直接寫入
		var lines []string
		for key, val := range cfg.envVars {
			lines = append(lines, fmt.Sprintf("%s=\"%s\"", key, val))
		}
		content := strings.Join(lines, "\n") + "\n"
		return os.WriteFile(targetFile, []byte(content), 0644)
	}

	// 基於範例檔案更新
	lines := strings.Split(string(data), "\n")
	var newContent strings.Builder
	written := make(map[string]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 保留註解和空行
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			fmt.Fprintln(&newContent, line)
			continue
		}

		// 處理環境變數
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])

			if val, ok := cfg.envVars[key]; ok && val != "" {
				fmt.Fprintf(&newContent, "%s=\"%s\"\n", key, val)
				written[key] = true
			} else {
				fmt.Fprintln(&newContent, line)
			}
		} else {
			fmt.Fprintln(&newContent, line)
		}
	}

	// 添加未寫入的變數
	for key, val := range cfg.envVars {
		if !written[key] && val != "" {
			fmt.Fprintf(&newContent, "%s=\"%s\"\n", key, val)
		}
	}

	return os.WriteFile(targetFile, []byte(newContent.String()), 0644)
}

func updateCaddyfile(cfg *Config) error {
	// 檢查 example.Caddyfile 是否存在
	if !fileExists(exampleCaddyfile) {
		return fmt.Errorf("%s 不存在", exampleCaddyfile)
	}

	// 如果 Caddyfile 已存在，先備份
	if fileExists(caddyfile) {
		if err := backupFile(caddyfile); err != nil {
			return err
		}
	}

	// 讀取範例檔案
	data, err := os.ReadFile(exampleCaddyfile)
	if err != nil {
		return err
	}

	// 替換內容
	content := string(data)
	content = strings.ReplaceAll(content, "your.domain.name", cfg.Domain)
	if cfg.Email != "" {
		content = strings.ReplaceAll(content, "your-email@domain.com", cfg.Email)
	}

	// 確保 data 目錄存在
	if err := os.MkdirAll("data", 0755); err != nil {
		return fmt.Errorf("無法建立 data 目錄: %w", err)
	}

	// 寫入檔案
	if err := os.WriteFile(caddyfile, []byte(content), 0644); err != nil {
		return err
	}

	green.Printf("✅ Caddyfile 已更新\n")
	return nil
}

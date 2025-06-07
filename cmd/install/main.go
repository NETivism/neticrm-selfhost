package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	exampleFile        = "example.env"
	targetFile         = ".env"
	defaultComposeFile = "docker-compose.yaml"
	sslComposeFile     = "docker-compose-ssl.yaml"
)

var reader = bufio.NewReader(os.Stdin)
var envVars = make(map[string]string) // 用於儲存環境變數

// backupFile 備份指定的檔案或資料夾
func backupFile(originalPath string) (string, error) {
	// 嘗試以 .bak, .bak1, .bak2... 為後綴進行備份
	backupPath := originalPath + ".bak"
	count := 0
	for {
		_, err := os.Stat(backupPath)
		if os.IsNotExist(err) {
			break
		}
		count++
		backupPath = fmt.Sprintf("%s.bak%d", originalPath, count)
	}

	// 檢查原始檔案是否存在
	_, err := os.Stat(originalPath)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("%s 不存在，無法備份", originalPath)
	}

	// 將檔案或資料夾改名為備份名稱
	err = os.Rename(originalPath, backupPath)
	if err != nil {
		return "", fmt.Errorf("無法備份 %s: %v", originalPath, err)
	}

	return backupPath, nil
}

// backupEnvFile 備份現有的 .env 檔案
func backupEnvFile() (string, error) {
	return backupFile(targetFile)
}

// checkMariaDBData 檢查 data/mariadb_data 資料夾是否存在且有資料
func checkMariaDBData() bool {
	mariadbPath := "data/mariadb_data"

	// 檢查資料夾是否存在
	info, err := os.Stat(mariadbPath)
	if os.IsNotExist(err) || !info.IsDir() {
		return false
	}

	// 檢查資料夾是否有檔案
	files, err := os.ReadDir(mariadbPath)
	if err != nil || len(files) == 0 {
		return false
	}

	return true
}

func main() {
	// First, check if .env exists
	if _, err := os.Stat(targetFile); err == nil {
		// 檢查是否有 data/mariadb_data 資料夾和檔案
		hasMariaDBData := checkMariaDBData()

		if hasMariaDBData {
			// 已有 data/mariadb_data 和檔案，詢問是否直接啟動
			fmt.Println("發現現有的資料庫檔案，看起來這是一個已經安裝好的網站。")
			fmt.Print("是否要直接啟動網站？(y/N) ")
			choice := strings.ToLower(readLine())
			if strings.HasPrefix(choice, "y") {
				// 檢查是否存在 data/Caddyfile 檔案
				_, err := os.Stat("data/Caddyfile")
				hasCaddyfile := err == nil // 如果沒有錯誤，表示檔案存在

				fmt.Println("正在啟動網站...")

				var cmd *exec.Cmd
				if hasCaddyfile {
					// 如果有 Caddyfile，使用 SSL 版本
					fmt.Println("(使用 SSL 配置啟動)")
					cmd = exec.Command("docker", "compose", "-f", "docker-compose-ssl.yaml", "up", "-d")
				} else {
					// 如果沒有 Caddyfile，使用預設版本
					fmt.Println("(使用非 SSL 配置啟動)")
					cmd = exec.Command("docker", "compose", "up", "-d")
				}

				cmd.Stdout = os.Stdout
				cmd.Stderr = os.Stderr
				if err := cmd.Run(); err != nil {
					fmt.Printf("啟動網站失敗: %v\n", err)
				} else {
					fmt.Println("網站已成功啟動！")
				}
				os.Exit(0)
			}
		}

		// 沒有資料庫檔案或用戶選擇不直接啟動
		fmt.Print("已發現 .env 檔案，是否要更改設定？(若要的話舊的 .env 檔會改名備份) (y/N) ")
		choice := strings.ToLower(readLine())

		if !strings.HasPrefix(choice, "y") {
			fmt.Println("安裝取消。")
			os.Exit(0)
		}

		// 備份現有的 .env 檔案
		backupFile, err := backupEnvFile()
		if err != nil {
			fmt.Printf("%v\n安裝取消。\n", err)
			os.Exit(1)
		}

		fmt.Printf("已將舊的 .env 檔案備份為 %s\n", backupFile)
	}

	// 1. Check if docker compose is available
	haveDocker := checkDockerCompose()

	// 載入預設環境變數
	loadDefaultEnvs()

	lang := chooseLanguage()

	if lang == "zh-hant" {
		fmt.Println("\n若您已有域名（Domain），請先將域名以A紀錄設到本主機 IP ，本安裝程式可自動幫您設定 SSL 並綁定網域")
		fmt.Println("或依照您所選的設定綁定特定埠（Port）")
	} else {
		fmt.Println("\nIf you already have a domain name, please point it to this host's IP address.")
		fmt.Println("This installer can automatically set up SSL and bind the domain for you,")
		fmt.Println("or bind to a specific port according to your chosen settings.")
	}
	domain, email, useSSL, port := askDomainAndSSL(lang)

	// Update .env file with domain and port if not using SSL
	if !useSSL {
		if domain != "" {
			updateEnv("DOMAIN", domain)
			updateEnv("PORT", "") // Clear port if domain is set
		} else {
			updateEnv("DOMAIN", "localhost") // Default domain if not set
			updateEnv("PORT", port)
		}
	} else {
		// For SSL, domain is handled by Caddyfile, clear PORT if it was set by default
		updateEnv("PORT", "")
	}

	// Update Caddyfile if SSL is used
	if useSSL {
		updateCaddyfile("your-domain.com", domain)
		updateCaddyfile("your-email@example.com", email)
	}

	mysqlConf := askMySQL(lang)
	for key, value := range mysqlConf {
		updateEnv(key, value)
	}

	adminUser, adminPass := askAdminCredentials(lang)
	updateEnv("ADMIN_LOGIN_USER", adminUser)
	updateEnv("ADMIN_LOGIN_PASSWORD", adminPass)
	updateEnv("LANGUAGE", lang)

	composeFile := "docker-compose.yaml"
	if useSSL {
		composeFile = "docker-compose-ssl.yaml"
	}

	// 在這裡生成 .env 檔案
	writeEnvFile()

	if lang == "zh-hant" {
		fmt.Printf("✅ .env 建立完成，開始 docker compose -f %s up -d ...\n", composeFile)
	} else {
		fmt.Println("✅ .env file created successfully.")
		fmt.Printf("Starting docker compose -f %s up -d ...\n", composeFile)
	}

	// Only run docker compose if it's available
	if haveDocker {
		dockerComposeUp(composeFile)
	} else {
		// This part also needs i18n if desired, but not specified in this request
		fmt.Println("Docker Compose is not available. Please install it and run manually:")
		fmt.Printf("docker compose -f %s up -d\n", composeFile)
	}
}

// ---------------- Utilities ----------------
func chooseLanguage() string {
	fmt.Println("1. What's your language?")
	fmt.Println(" 1) en")
	fmt.Println(" 2) 台灣繁體中文")
	for {
		fmt.Print("Please enter 1 or 2: ")
		choice := readLine()
		if choice == "1" {
			return "en"
		} else if choice == "2" {
			return "zh-hant"
		}
	}
}

func askDomainAndSSL(lang string) (string, string, bool, string) {
	var domain, email, port string
	var useSSL bool

	// Ask about SSL first
	var sslPrompt string
	if lang == "zh-hant" {
		sslPrompt = "您是否有網域並希望自動設定 SSL？ [y/N] "
	} else {
		sslPrompt = "Do you have a domain and want to set up SSL automatically? [y/N] "
	}
	fmt.Print(sslPrompt)
	choice := strings.ToLower(readLine())
	useSSL = strings.HasPrefix(choice, "y")

	if useSSL {
		// SSL Path
		var domainPrompt, emailPrompt string
		if lang == "zh-hant" {
			domainPrompt = "請輸入您的域名 (例如 example.com): "
			emailPrompt = "請輸入您的電子郵件 (用於 Let's Encrypt SSL 證書): "
		} else {
			domainPrompt = "Please enter your domain name (e.g., example.com): "
			emailPrompt = "Please enter your email (for Let's Encrypt SSL certificate): "
		}
		fmt.Print(domainPrompt)
		domain = readLine()
		if domain == "" {
			// Domain is mandatory for SSL
			if lang == "zh-hant" {
				fmt.Println("域名 SSL 模式下不可為空")
			} else {
				fmt.Println("Domain cannot be empty for SSL")
			}
			os.Exit(1)
		}
		fmt.Print(emailPrompt)
		email = readLine()
		port = "" // Port is not used with SSL directly in .env, Caddy handles it
	} else {
		// Non-SSL Path
		domainPrompt := ""
		if lang == "zh-hant" {
			domainPrompt = "網站網址 (domain，可留空): "
		} else {
			domainPrompt = "Domain (leave blank for no domain): "
		}
		fmt.Print(domainPrompt)
		domain = readLine()

		if domain == "" {
			portPrompt := ""
			if lang == "zh-hant" {
				portPrompt = "請輸入 Port (預設 8080): "
			} else {
				portPrompt = "Please enter Port (default 8080): "
			}
			fmt.Print(portPrompt)
			port = readLine()
			if port == "" {
				port = "8080"
			}
		} else {
			port = "" // If domain is set, port is usually handled by reverse proxy or not needed in .env
		}
		email = ""
	}
	return domain, email, useSSL, port
}

func askMySQLPassword(lang string, passwordName string) string {
	var passPrompt, confirmPrompt, mismatchMsg, reEnterPrompt string
	if lang == "zh-hant" {
		passPrompt = passwordName + " (留空自動產生): "
		confirmPrompt = "請再次輸入" + passwordName + "密碼確認: "
		mismatchMsg = "   ✗ 兩次密碼不一致，請重新輸入。"
		reEnterPrompt = "重新輸入" + passwordName + ": "
	} else {
		passPrompt = passwordName + " (leave blank for random password): "
		confirmPrompt = "Please re-enter " + passwordName + " to confirm: "
		mismatchMsg = "   ✗ Passwords do not match. Please re-enter."
		reEnterPrompt = "Re-enter " + passwordName + ": "
	}

	pass := readPassword(passPrompt)
	if pass == "" {
		return randomPass(13) // Auto-generate if blank, do not print
	}
	for {
		confirm := readPassword(confirmPrompt)
		if pass == confirm {
			return pass
		}
		fmt.Println(mismatchMsg)
		pass = readPassword(reEnterPrompt)
	}
}

func askMySQL(lang string) map[string]string {
	conf := map[string]string{}

	if lang == "zh-hant" {
		fmt.Print("是否要修改 MySQL 參數？(y/N) ")
	} else {
		fmt.Print("Modify MySQL parameters? (y/N) ")
	}
	choice := strings.ToLower(readLine())

	if !strings.HasPrefix(choice, "y") { // User chose N or pressed Enter
		// 自動產生密碼，但保留預設的其他字段
		conf["MYSQL_ROOT_PASSWORD"] = randomPass(13)
		conf["MYSQL_DATABASE"] = "" // Mark to retain the example value
		conf["MYSQL_USER"] = ""     // Mark to retain the example value
		conf["MYSQL_PASSWORD"] = randomPass(13)
	} else { // User chose Y
		// ROOT 密碼
		conf["MYSQL_ROOT_PASSWORD"] = askMySQLPassword(lang, "MYSQL_ROOT_PASSWORD")

		// 資料庫名稱
		if lang == "zh-hant" {
			fmt.Print("MYSQL_DATABASE (留空使用預設值): ")
		} else {
			fmt.Print("MYSQL_DATABASE (leave blank for default): ")
		}
		if v := readLine(); v != "" {
			conf["MYSQL_DATABASE"] = v
		}

		// 用戶名稱
		if lang == "zh-hant" {
			fmt.Print("MYSQL_USER (留空使用預設值): ")
		} else {
			fmt.Print("MYSQL_USER (leave blank for default): ")
		}
		if v := readLine(); v != "" {
			conf["MYSQL_USER"] = v
		}

		// 用戶密碼
		conf["MYSQL_PASSWORD"] = askMySQLPassword(lang, "MYSQL_PASSWORD")
	}

	return conf // Now always returns a map
}

func askAdminCredentials(lang string) (string, string) {
	user := askUsername(lang)
	pass := askPassword(lang)
	return user, pass
}

func askUsername(lang string) string {
	var prompt string
	if lang == "zh-hant" {
		prompt = "ADMIN_LOGIN_USER (留空自動產生): "
	} else {
		prompt = "ADMIN_LOGIN_USER (leave blank for example value): "
	}
	fmt.Print(prompt)
	user := readLine()
	if user == "" {
		return "example" // Default to 'example' if blank
	}
	return user
}

func askPassword(lang string) string {
	var passPrompt, confirmPrompt, mismatchMsg, reEnterPrompt string
	if lang == "zh-hant" {
		passPrompt = "ADMIN_LOGIN_PASSWORD (留空自動產生): "
		confirmPrompt = "請再次輸入密碼確認: "
		mismatchMsg = "   ✗ 兩次不一致，請重新輸入。"
		reEnterPrompt = "重新輸入密碼: "
	} else {
		passPrompt = "ADMIN_LOGIN_PASSWORD (leave blank and the installer will provide a random password): "
		confirmPrompt = "Please re-enter password to confirm: "
		mismatchMsg = "   ✗ Passwords do not match. Please re-enter."
		reEnterPrompt = "Re-enter password: "
	}

	pass := readPassword(passPrompt)
	if pass == "" {
		return randomPass(11) // Auto-generate if blank, do not print
	}
	for {
		confirm := readPassword(confirmPrompt)
		if pass == confirm {
			return pass
		}
		fmt.Println(mismatchMsg)
		pass = readPassword(reEnterPrompt) // Use the translated re-enter prompt
	}
}

// --------- File and String Processing ---------
func dockerComposeUp(composeFile string) {
	cmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Error starting docker compose: %v\n", err)
		// Decide if to os.Exit(1) or let the user know and continue
	}
}

// 不再使用 copyFile 函數

func updateEnv(key, val string) {
	// 如果值為空，則不更新環境變數
	if val == "" {
		return
	}

	// 將鍵值對存入映射
	envVars[key] = val
}

// 載入預設環境變數
func loadDefaultEnvs() {
	data, err := os.ReadFile(exampleFile)
	if err != nil {
		fmt.Printf("Error reading example file %s: %v\n", exampleFile, err)
		return
	}

	// 分割檔案內容為行
	lines := strings.Split(string(data), "\n")

	// 解析每一行，提取環境變數
	for _, line := range lines {
		// 忽略註解行和空行
		if strings.HasPrefix(strings.TrimSpace(line), "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// 分割 key=value
		parts := strings.SplitN(strings.TrimSpace(line), "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			val := strings.TrimSpace(parts[1])
			// 預設值加入到環境變數映射
			envVars[key] = val
		}
	}
}

// 將收集到的環境變數寫入 .env 檔案
func writeEnvFile() {
	// 讀取 example.env 以保留註釋和結構
	data, err := os.ReadFile(exampleFile)
	if err != nil {
		fmt.Printf("Error reading example file %s: %v\n", exampleFile, err)

		// 如果無法讀取範例檔，則直接寫入環境變數
		var content strings.Builder
		for key, val := range envVars {
			fmt.Fprintf(&content, "%s=%s\n", key, val)
		}

		// 寫入檔案
		err = os.WriteFile(targetFile, []byte(content.String()), 0644)
		if err != nil {
			fmt.Printf("Error writing file %s: %v\n", targetFile, err)
		}
		return
	}

	// 分割檔案內容為行
	lines := strings.Split(string(data), "\n")
	var newContent strings.Builder

	// 處理每一行
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// 保留註解行和空行
		if strings.HasPrefix(trimmedLine, "#") || trimmedLine == "" {
			fmt.Fprintln(&newContent, line)
			continue
		}

		// 處理環境變數行
		parts := strings.SplitN(trimmedLine, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])

			// 如果我們有用戶提供的值，使用它
			if val, ok := envVars[key]; ok {
				fmt.Fprintf(&newContent, "%s=%s\n", key, val)
				// 從映射中刪除，這樣我們可以跟踪哪些變數尚未寫入
				delete(envVars, key)
			} else {
				// 保留原始行
				fmt.Fprintln(&newContent, line)
			}
		} else {
			// 不是環境變數格式，保留原始行
			fmt.Fprintln(&newContent, line)
		}
	}

	// 添加任何剩餘的環境變數
	for key, val := range envVars {
		fmt.Fprintf(&newContent, "%s=%s\n", key, val)
	}

	// 寫入檔案
	err = os.WriteFile(targetFile, []byte(newContent.String()), 0644)
	if err != nil {
		fmt.Printf("Error writing file %s: %v\n", targetFile, err)
	}
}

func randomPass(length int) string {
	buf := make([]byte, length)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)[:length]
}

// --------- Environment and File Handling Utilities ---------
func checkDockerCompose() bool {
	cmd := exec.Command("docker", "compose", "version")
	err := cmd.Run()
	return err == nil
}

func updateCaddyfile(oldValue, newValue string) {
	filePath := "data/Caddyfile"

	// Read file
	data, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Printf("Error reading %s: %v\n", filePath, err)
		return
	}

	// Replace content
	newContent := strings.ReplaceAll(string(data), oldValue, newValue)

	// Write back to file
	err = os.WriteFile(filePath, []byte(newContent), 0644)
	if err != nil {
		fmt.Printf("Error writing %s: %v\n", filePath, err)
	}
}

// --------- Interactive Input ---------
func readLine() string {
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func readPassword(prompt string) string {
	fmt.Print(prompt)

	// 嘗試使用 stty 來隱藏密碼輸入，這在 Linux/Mac 環境下比較可靠
	cmd := exec.Command("bash", "-c", "stty -echo; cat; stty echo")
	cmd.Stdin = os.Stdin
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		fmt.Printf("\n無法建立安全的密碼輸入: %v\n", err)
		return readLine()
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("\n無法建立安全的密碼輸入: %v\n", err)
		return readLine()
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("\n無法建立安全的密碼輸入: %v\n", err)
		return readLine()
	}

	password, err := bufio.NewReader(stdout).ReadString('\n')
	fmt.Println() // 添加換行，因為 stty -echo 不會自動換行

	// 關閉 stdin 來結束命令
	stdin.Close()

	err = cmd.Wait()
	if err != nil {
		// 如果密碼輸入失敗，則將密碼輸入設置為可見並重試
		fmt.Printf("\n無法安全地讀取密碼: %v\n請純簡輸入: ", err)
		return readLine()
	}

	// 移除尾端的換行符
	password = strings.TrimSpace(password)
	return password
}

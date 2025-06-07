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

func main() {
	// First, check if .env exists
	if _, err := os.Stat(targetFile); err == nil {
		fmt.Println(".env file already exists. Please remove it if you want to re-run the installer.")
		fmt.Println(".env 檔案已存在，請移除後再重新執行安裝程式。")
		os.Exit(1)
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
			domainPrompt = "2. 網站網址 (domain，可留空): "
		} else {
			domainPrompt = "2. Domain (leave blank for no domain): "
		}
		fmt.Print(domainPrompt)
		domain = readLine()

		if domain == "" {
			portPrompt := ""
			if lang == "zh-hant" {
				portPrompt = "2-1. 請輸入 Port (預設 8080): "
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

func askMySQL(lang string) map[string]string {
	conf := map[string]string{}
	if lang == "zh-hant" {
		fmt.Print("3. 是否要修改 MySQL 參數？(y/N) ")
	} else {
		fmt.Print("3. Modify MySQL parameters? (y/N) ")
	}
	choice := strings.ToLower(readLine())

	if !strings.HasPrefix(choice, "y") { // User chose N or pressed Enter
		conf["MYSQL_ROOT_PASSWORD"] = randomPass(13)
		conf["MYSQL_DATABASE"] = "" // Mark to retain the example value
		conf["MYSQL_USER"] = ""     // Mark to retain the example value
		conf["MYSQL_PASSWORD"] = randomPass(13)
	} else { // User chose Y
		if lang == "zh-hant" {
			fmt.Print("3-1. MYSQL_ROOT_PASSWORD (留空自動產生): ")
		} else {
			fmt.Print("3-1. MYSQL_ROOT_PASSWORD (leave blank for random password): ")
		}
		if v := readLine(); v != "" {
			conf["MYSQL_ROOT_PASSWORD"] = v
		} else {
			conf["MYSQL_ROOT_PASSWORD"] = randomPass(13)
		}
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
		prompt = "4. ADMIN_LOGIN_USER (留空自動產生): "
	} else {
		prompt = "4. ADMIN_LOGIN_USER (leave blank for example value): "
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
		passPrompt = "5. ADMIN_LOGIN_PASSWORD (留空自動產生): "
		confirmPrompt = "5.1 請再次輸入密碼確認: "
		mismatchMsg = "   ✗ 兩次不一致，請重新輸入。"
		reEnterPrompt = "5. 重新輸入密碼: "
	} else {
		passPrompt = "5. ADMIN_LOGIN_PASSWORD (leave blank and the installer will provide a random password): "
		confirmPrompt = "5.1 Please re-enter password to confirm: "
		mismatchMsg = "   ✗ Passwords do not match. Please re-enter."
		reEnterPrompt = "5. Re-enter password: "
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

func askLine(prompt string) string { fmt.Print(prompt); return readLine() }

func readPassword(prompt string) string {
	// On Windows, exec.Command("bash", "-c", "read -rs ...") is unlikely to work
	// without a specific setup like WSL and bash in PATH.
	// For now, using a simple readLine to ensure the prompt waits for input.
	// This will not hide the password input.
	fmt.Print(prompt) // Prompt is from the caller
	return readLine()
}

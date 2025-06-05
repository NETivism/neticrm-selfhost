package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
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

func main() {
	// First, copy example.env to .env to ensure a base file exists
	copyFile(exampleFile, targetFile)

	// 1. Check if docker compose is available
	haveDocker := checkDockerCompose()

	// check if .env exists
	if _, err := os.Stat(targetFile); err == nil {
		fmt.Println("\n.env file already exists. Please remove it if you want to re-run the installer.")
		os.Exit(1)
	}

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
		prompt = "4. ADMIN_LOGIN_USER (leave blank for example value): "
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

func copyFile(src, dst string) {
	in, _ := os.ReadFile(src)
	_ = os.WriteFile(dst, in, 0644)
}

func updateEnv(key, val string) {
	// If the value is empty, keep the original value (do not update)
	if val == "" {
		return
	}

	// Read the entire file content
	data, err := os.ReadFile(targetFile)
	if err != nil {
		fmt.Printf("Error reading file %s: %v\n", targetFile, err)
		return
	}

	// Split file content into lines
	lines := strings.Split(string(data), "\n")

	// Find and update matching lines
	found := false
	for i, line := range lines {
		// Ignore comment lines and empty lines
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// Check if it's the target variable
		if strings.HasPrefix(line, key+"=") || strings.HasPrefix(line, key+" =") {
			lines[i] = fmt.Sprintf("%s=%s", key, val)
			found = true
			break
		}
	}

	// If no matching line is found, add a new line
	if !found {
		lines = append(lines, fmt.Sprintf("%s=%s", key, val))
	}

	// Write the updated content back to the file
	out := []byte(strings.Join(lines, "\n"))
	err = os.WriteFile(targetFile, out, 0644)
	if err != nil {
		fmt.Printf("Error writing file %s: %v\n", targetFile, err)
	}
}

// --------- Random Value Utilities ---------
func randomPort() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("%d", n.Int64()+50001)
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
	filePath := "Caddyfile"

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

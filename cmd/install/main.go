package main

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"regexp"
	"strings"
	
)

const (
	exampleFile = "example.env"
	targetFile  = ".env"
)

var reader = bufio.NewReader(os.Stdin)

func main() {
	lang := chooseLanguage()
	domain, port := askDomainAndPort()
	mysqlConf := askMySQL()
	adminUser := askLine("4. ADMIN_LOGIN_USER (留空採用範例): ")
	adminPass := askPassword()

	copyFile(exampleFile, targetFile)
	updateEnv("LANGUAGE", lang)
	updateEnv("DOMAIN", domain)
	updateEnv("HTTP_PORT", port)

	if mysqlConf != nil {
		updateEnv("MYSQL_ROOT_PASSWORD", mysqlConf["root"])
		if mysqlConf["db"] != "" {
			updateEnv("MYSQL_DATABASE", mysqlConf["db"])
		}
		if mysqlConf["user"] != "" {
			updateEnv("MYSQL_USER", mysqlConf["user"])
		}
		updateEnv("MYSQL_PASSWORD", mysqlConf["pass"])
	}

	if adminUser != "" {
		updateEnv("ADMIN_LOGIN_USER", adminUser)
	}
	updateEnv("ADMIN_LOGIN_PASSWORD", adminPass)

	fmt.Println("✅ .env 建立完成，開始 docker compose up -d ...")
	// cmd := exec.Command("docker", "compose", "up", "-d")
	// cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	// _ = cmd.Run()
}

// ---------------- 小工具 ----------------
func chooseLanguage() string {
	fmt.Println("1. What's your language?")
	fmt.Println(" 1) en")
	fmt.Println(" 2) zh-hant")
	for {
		fmt.Print("請輸入 1 或 2: ")
		choice := readLine()
		if choice == "1" {
			return "en"
		} else if choice == "2" {
			return "zh-hant"
		}
	}
}

func askDomainAndPort() (string, string) {
	fmt.Print("2. 網站網址 (domain，可留空): ")
	domain := readLine()
	if domain == "" {
		fmt.Print("2-1. 請輸入 Port (預設 8080): ")
		port := readLine()
		if port == "" {
			port = "8080"
		}
		return "", port
	}
	return domain, randomPort()
}

func askMySQL() map[string]string {
	fmt.Print("3. 是否要修改 MySQL 參數？(y/N) ")
	choice := strings.ToLower(readLine())
	if !strings.HasPrefix(choice, "y") {
		return nil
	}
	conf := map[string]string{}
	fmt.Print("3-1. MYSQL_ROOT_PASSWORD (留空自動產生): ")
	if v := readLine(); v != "" {
		conf["root"] = v
	} else {
		conf["root"] = randomPass(13)
	}
	fmt.Print("3-2. MYSQL_DATABASE (留空保留範例): ")
	conf["db"] = readLine()

	fmt.Print("3-3. MYSQL_USER (留空保留範例): ")
	conf["user"] = readLine()

	fmt.Print("3-4. MYSQL_PASSWORD (留空自動產生): ")
	if v := readLine(); v != "" {
		conf["pass"] = v
	} else {
		conf["pass"] = randomPass(13)
	}
	return conf
}

func askPassword() string {
	pass := readPassword("5. ADMIN_LOGIN_PASSWORD (留空自動產生): ")
	if pass == "" {
		pass = randomPass(11)
		fmt.Println("   → 隨機產生：", pass)
		return pass
	}
	for {
		confirm := readPassword("5.1 請再次輸入密碼確認: ")
		if pass == confirm {
			return pass
		}
		fmt.Println("   ✗ 兩次不一致，請重新輸入。")
		pass = readPassword("5. 重新輸入密碼: ")
	}
}

// --------- 檔案與字串處理 ---------
func copyFile(src, dst string) {
	in, _ := os.ReadFile(src)
	_ = os.WriteFile(dst, in, 0644)
}

func updateEnv(key, val string) {
	inputBytes, err := os.ReadFile(targetFile)
	if err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "讀取檔案 %s 錯誤: %v\n", targetFile, err)
		return
	}

	content := string(inputBytes)
	regex := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `=.*`)

	var newContent string
	if regex.MatchString(content) {
		newContent = regex.ReplaceAllString(content, fmt.Sprintf("%s=%s", key, val))
	} else {
		if content != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		newContent = content + fmt.Sprintf("%s=%s\n", key, val)
	}

	if newContent != "" {
		newContent = strings.TrimRight(newContent, "\n") + "\n"
	}

	err = os.WriteFile(targetFile, []byte(newContent), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "寫入檔案 %s 錯誤: %v\n", targetFile, err)
	}
}

// --------- 亂數工具 ---------
func randomPort() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(10000))
	return fmt.Sprintf("%d", n.Int64()+50001)
}

func randomPass(length int) string {
	buf := make([]byte, length)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)[:length]
}

// --------- 互動輸入 ---------
func readLine() string {
	text, _ := reader.ReadString('\n')
	return strings.TrimSpace(text)
}

func askLine(prompt string) string { fmt.Print(prompt); return readLine() }

func readPassword(prompt string) string {
	fmt.Print(prompt)
	bytePw, _ := exec.Command("bash", "-c", "read -rs pw; echo $pw").Output()
	fmt.Println()
	return strings.TrimSpace(string(bytePw))
}


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
	if !haveDocker {
		fmt.Println("\nWarning: docker compose command not found. Only .env file will be updated, services will not be started automatically.\n")
		fmt.Print("Continue? [Y/n] ")
		choice := strings.ToLower(readLine())
		if choice == "n" {
			fmt.Println("Installation cancelled.")
			return
		}
	}

	// 2. Ask for language preference first
	lang := chooseLanguage()

	// 3. Ask if SSL should be configured
	composeFile := defaultComposeFile
	domainName := ""
	useSSL := false

	fmt.Print("Do you have a domain and want to set up SSL automatically? [y/N] ")
	choice := strings.ToLower(readLine())
	if strings.HasPrefix(choice, "y") {
		useSSL = true
		composeFile = sslComposeFile

		// Get domain name and email
		fmt.Print("Please enter your domain name (e.g., example.com): ")
		domainName = readLine()

		// Update Caddyfile
		updateCaddyfile("your.domain.name", domainName)

		fmt.Print("Please enter your email (for Let's Encrypt SSL certificate): ")
		email := readLine()
		updateCaddyfile("your-email@domain.com", email)
	}

	// 4. Proceed with the rest of the installation flow
	var domain string
	var port string

	// If a domain name was already provided during SSL setup, don't ask again
	if useSSL && domainName != "" {
		// Domain name already provided, use it directly
		domain = domainName
		port = "443" // SSL defaults to port 443
	} else {
		// Otherwise, ask for domain and port
		domain, port = askDomainAndPort()
	}

	mysqlConf := askMySQL()
	adminUser := askLine("4. ADMIN_LOGIN_USER (leave blank for example value): ")

	// 5. Modify ADMIN_LOGIN_PASSWORD handling
	adminPass := readPassword("5. ADMIN_LOGIN_PASSWORD (leave blank and the installer will provide a random password): ")

	// Update environment variables, only if the value is not empty
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

	// Only update if the admin password is not empty
	if adminPass != "" {
		updateEnv("ADMIN_LOGIN_PASSWORD", adminPass)
	}
	// If admin password is empty, keep the empty value in .env; the installer will auto-provide a random password later

	fmt.Println("✅ .env file created successfully.")

	// If docker compose is available, start it
	if haveDocker {
		fmt.Printf("Starting docker compose -f %s up -d ...\n", composeFile)
		cmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		_ = cmd.Run()
	} else {
		fmt.Println("Please manually run docker compose to start the services.")
	}
}

// ---------------- Utilities ----------------
func chooseLanguage() string {
	fmt.Println("1. What's your language?")
	fmt.Println(" 1) en")
	fmt.Println(" 2) zh-hant")
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

func askDomainAndPort() (string, string) {
	fmt.Print("2. Website domain (optional): ")
	domain := readLine()
	if domain == "" {
		fmt.Print("2-1. Please enter Port (default 8080): ")
		port := readLine()
		if port == "" {
			port = "8080"
		}
		return "", port
	}
	return domain, randomPort()
}

func askMySQL() map[string]string {
	conf := map[string]string{} // Initialize map

	fmt.Print("3. Modify MySQL parameters? (y/N) ")
	choice := strings.ToLower(readLine())

	if !strings.HasPrefix(choice, "y") { // User chose N or pressed Enter
		conf["root"] = randomPass(13)
		conf["db"] = ""   // Mark to retain the example value
		conf["user"] = "" // Mark to retain the example value
		conf["pass"] = randomPass(13)
	} else { // User chose Y
		fmt.Print("3-1. MYSQL_ROOT_PASSWORD (leave blank to auto-generate): ")
		if v := readLine(); v != "" {
			conf["root"] = v
		} else {
			conf["root"] = randomPass(13)
		}

		fmt.Print("3-2. MYSQL_DATABASE (leave blank to keep example value): ")
		conf["db"] = readLine() // If empty, the main function will skip updateEnv

		fmt.Print("3-3. MYSQL_USER (leave blank to keep example value): ")
		conf["user"] = readLine() // If empty, the main function will skip updateEnv

		fmt.Print("3-4. MYSQL_PASSWORD (leave blank to auto-generate): ")
		if v := readLine(); v != "" {
			conf["pass"] = v
		} else {
			conf["pass"] = randomPass(13)
		}
	}
	return conf // Now always returns a map
}

func askPassword() string {
	pass := readPassword("5. ADMIN_LOGIN_PASSWORD (leave blank and the installer will provide a random password): ")
	if pass == "" {
		return "" // Return empty string if user leaves it blank
	}
	for {
		confirm := readPassword("5.1 Please re-enter password to confirm: ")
		if pass == confirm {
			return pass
		}
		fmt.Println("   ✗ Passwords do not match. Please re-enter.")
		pass = readPassword("5. Re-enter password: ")
	}
}

// --------- File and String Processing ---------
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

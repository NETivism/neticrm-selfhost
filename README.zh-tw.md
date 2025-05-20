# netiCRM 自建主機安裝指南

## 系統需求
- Docker
- Docker compose

## 安裝步驟

1. **複製程式碼庫：**
    ```sh
    git clone https://github.com/netivism/neticrm-selfhost
    cd neticrm-selfhost
    ```

2. **複製環境變數設定檔並進行設定：**
    ```sh
    cp example.env .env
    ```

3. **編輯 `.env` 檔案**以設定您的環境變數：
    ```sh
    nano .env
    ```
    請務必更新 `MYSQL_ROOT_PASSWORD`、`MYSQL_DATABASE`、`MYSQL_USER` 和 `MYSQL_PASSWORD` 為您自己的值。同時，也要更改 `ADMIN_LOGIN_USER` 和 `ADMIN_LOGIN_PASSWORD` 以防止他人以管理員身分登入。

4. **啟動 Docker 容器：**
    ```sh
    docker compose up -d
    ```

5. **存取應用程式：**
    安裝完成後，打開您的網頁瀏覽器並前往 `http://localhost:8080`（或您在 `.env` 檔案中設定的端口）。

6. **登入系統：**
    有兩種方式可以取得登入使用者名稱和密碼：
    - 使用 `.env` 檔案中的 `ADMIN_LOGIN_USER` 和 `ADMIN_LOGIN_PASSWORD` 進行登入。
    - 使用以下指令生成一次性登入連結：
      ```sh
      docker exec -it neticrm-php bash -c 'drush -l $DOMAIN uli'
      ```

7. **按照畫面上的指示**完成設定。

## 使用 Caddy 設定 SSL

對於生產環境，建議使用 SSL。此程式碼庫包含一個 `docker-compose-ssl.yaml` 組態，使用 Caddy 作為反向代理自動處理 SSL。

1. **設定您的 Caddyfile：**
    ```sh
    # 編輯 Caddyfile，加入您的網域和電子郵件
    nano Caddyfile
    ```
    
    Caddyfile 內容範例：
    ```
    {
        email your-email@domain.com
    }
    your.domain.name {
        reverse_proxy neticrm-nginx:80
    }
    ```
    
    請將 `your-email@domain.com` 替換為您的電子郵件地址，將 `your.domain.name` 替換為您實際的網域。

2. **啟動啟用 SSL 的堆疊：**
    ```sh
    docker compose -f docker-compose-ssl.yaml up -d
    ```

3. **存取您的網站：**
    您的網站現在應該可以通過 `https://your.domain.name` 存取，並擁有由 Caddy 自動獲取的有效 SSL 憑證。

## 停止容器
若要停止正在運行的容器，請使用：
```sh
docker compose down
```

## 其他指令
- **查看日誌：**
    ```sh
    docker compose logs -f
    ```
- **重啟服務：**
    ```sh
    docker compose restart
    ```

如需更詳細的資訊，請參考官方文件或聯繫技術支援。

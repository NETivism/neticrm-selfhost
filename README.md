# netiCRM Self-Host Installation Guide

## Prerequisites
- Docker
- Docker compose

## Quick Install

You can simply run the installer script and follow the on-screen instructions:

```sh
./install
```

This will guide you through all necessary configuration and setup steps interactively.

## Installation Steps

1. **Clone the repository:**
    ```sh
    git clone https://github.com/netivism/neticrm-selfhost
    cd neticrm-selfhost
    ```

2. **Copy the example environment file and configure it:**
    ```sh
    cp example.env .env
    ```

3. **Edit the `.env` file** to set your own environment variables:
    ```sh
    nano .env
    ```
    Make sure to update the `MYSQL_ROOT_PASSWORD`, `MYSQL_DATABASE`, `MYSQL_USER`, and `MYSQL_PASSWORD` with your own values. Also, change `ADMIN_LOGIN_USER` and `ADMIN_LOGIN_PASSWORD` to prevent others from logging in as the administrator.

    4. **Start the Docker containers:**
    ```sh
    docker compose up -d
    ```

5. **Access the application:**
    After a while of installation. Open your web browser and navigate to `http://localhost:8080` (or the port you configured in the `.env` file).

6. **Login to the system:**
    There are two ways to get login user and password:
    - Use `ADMIN_LOGIN_USER` and `ADMIN_LOGIN_PASSWORD` in `.env` file to login.
    - Generate a one-time login link using the following command:
      ```sh
      docker exec -it neticrm-php bash -c 'drush -l $DOMAIN uli'
      ```

7. **Follow the on-screen instructions** to complete the setup.

## SSL Setup with Caddy

For production environments, it's recommended to use SSL. This repository includes a `docker-compose-ssl.yaml` configuration that uses Caddy as a reverse proxy to handle SSL automatically.

1. **Configure your Caddyfile:**

    Rename or copy the example configuration file:

    ```sh
    cp data/example.Caddyfile data/Caddyfile
   
    # Edit the Caddyfile with your domain and email
    nano data/Caddyfile
    ```

    Example Caddyfile content:
    ```
    {
        email your-email@domain.com
    }
    your.domain.name {
        reverse_proxy neticrm-nginx:80
    }
    ```

    Replace `your-email@domain.com` with your email address and `your.domain.name` with your actual domain.

2. **Start the SSL-enabled stack:**
    ```sh
    docker compose -f docker-compose-ssl.yaml up -d
    ```

3. **Access your site:**
    Your site should now be accessible at `https://your.domain.name` with a valid SSL certificate automatically obtained by Caddy.

## Stopping the Containers
To stop the running containers, use:
```sh
docker compose down
```

## Additional Commands
- **View logs:**
    ```sh
    docker compose logs -f
    ```
- **Restart services:**
    ```sh
    docker compose restart
    ```

For more detailed information, refer to the official documentation or contact support.

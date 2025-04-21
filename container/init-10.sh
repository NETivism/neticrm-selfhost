#!/bin/bash
echo "Checking for database connection..."
while ! mysqladmin ping -h mariadb -u "$MYSQL_USER" -p"$MYSQL_PASSWORD" --silent; do
  echo "Database not ready, waiting..."
  sleep 3
done
echo "Database is running and accepting connections."

get_latest_neticrm_version() {
  # Use GitHub API to get the latest release tag
  latest_version=$(curl -s https://api.github.com/repos/NETivism/netiCRM/releases/latest | grep "tag_name" | cut -d '"' -f 4)

  # If API call fails, try to parse from the releases page
  if [ -z "$latest_version" ]; then
    latest_version=$(curl -s https://github.com/NETivism/netiCRM/releases | grep -o 'NETivism/netiCRM/releases/tag/[0-9]*\.[0-9]*\.[0-9]*' | head -1 | cut -d '/' -f 6)
  fi

  # Remove 'v' prefix if present
  latest_version=${latest_version#v}

  echo "$latest_version"
}

export DRUPAL=10
export DRUPAL_ROOT=/var/www/html
if ! grep -q "export TERM=xterm" /root/.bashrc; then
  echo "export TERM=xterm" >> /root/.bashrc
fi
if ! grep -q "export DRUPAL_ROOT=/var/www/html" /root/.bashrc; then
  echo "export DRUPAL_ROOT=/var/www/html" >> /root/.bashrc
fi
if ! grep -q "export DOMAIN" /root/.bashrc; then
  echo "export DOMAIN=$DOMAIN" >> /root/.bashrc
fi



if mysql -h mariadb -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -e "SHOW DATABASES;" > /dev/null 2>&1; then
  echo "✅ Connection successful!"
else
  echo "❌ Connection failed!"
  echo "Error message:"
  mysql -h mariadb -u"$MYSQL_USER" -p"$MYSQL_PASSWORD" -e "SHOW DATABASES;" 2>&1
fi


date +"@ %Y-%m-%d %H:%M:%S %z"
echo "Downloading Drupal-$DRUPAL + netiCRM"
if [ ! -d $DRUPAL_ROOT/modules/civicrm ]; then
  NETICRM_VERSION=$(get_latest_neticrm_version)
  if [ -z "$NETICRM_VERSION" ]; then
    echo "Error: Could not determine the latest version."
    exit 1
  fi
  echo "Latest netiCRM version: $NETICRM_VERSION"
  DOWNLOAD_URL="https://github.com/NETivism/netiCRM/releases/download/$NETICRM_VERSION/neticrm-$NETICRM_VERSION.tar.gz"
  DOWNLOAD_PATH="/tmp/neticrm-$NETICRM_VERSION.tar.gz"
  echo "Downloading netiCRM from: $DOWNLOAD_URL"
  curl -L -o "$DOWNLOAD_PATH" "$DOWNLOAD_URL"

  # Check if download was successful
  if [ ! -f "$DOWNLOAD_PATH" ]; then
    echo "Error: Download failed."
    exit 1
  fi
  mkdir -p "$DRUPAL_ROOT/modules/civicrm"
  tar -xzf "$DOWNLOAD_PATH" -C "$DRUPAL_ROOT/modules/civicrm" --strip-components=1

  # Clean up
  echo "Cleaning up..."
  rm "$DOWNLOAD_PATH"
  echo "netiCRM $VERSION has been successfully downloaded to $DRUPAL_ROOT/modules/civicrm"
fi

date +"@ %Y-%m-%d %H:%M:%S %z"
echo "Installing Drupal-$DRUPAL + netiCRM"

if [ -f $DRUPAL_ROOT/sites/default/civicrm.settings.php ]; then
  echo "civicrm.settings.php is existed, do not install Drupal and CiviCRM."
else
  cd $DRUPAL_ROOT
  echo "Install Drupal ..."
  date +"@ %Y-%m-%d %H:%M:%S %z"
  sleep 5s
  drush -vvvv --yes site-install standard --locale=${LANGUAGE:-en} --account-name=$ADMIN_LOGIN_USER --db-url=mysql://$MYSQL_USER:${MYSQL_PASSWORD}@mariadb/$MYSQL_DATABASE --account-pass=$ADMIN_LOGIN_PASSWORD --site-name=netiCRM

  if [ -f $DRUPAL_ROOT/sites/default/settings.php ]; then
    if ! grep -q "date_default_timezone_set" $DRUPAL_ROOT/sites/default/settings.php; then
      echo 'date_default_timezone_set("Asia/Taipei");' >> $DRUPAL_ROOT/sites/default/settings.php
      echo 'ini_set("error_reporting", E_ALL & ~E_NOTICE & ~E_STRICT & ~E_DEPRECATED & ~E_WARNING);' >> $DRUPAL_ROOT/sites/default/settings.php
      echo "\$base_url='';" >> $DRUPAL_ROOT/sites/default/settings.php
      echo "\$settings['civicrm_demo.sample_data_ci'] = TRUE;" >> $DRUPAL_ROOT/sites/default/settings.php
      echo "\$config['system.performance']['js']['preprocess'] = FALSE;" >> $DRUPAL_ROOT/sites/default/settings.php
    fi
  fi

  drush --yes pm:install civicrm
  drush --yes pm:install civicrm_allpay
  drush --yes pm:install civicrm_spgateway
  drush --yes pm:install neticrm_drush
  drush --yes pm:install civicrm_demo
  drush --yes pm:install neticrm_update
#  composer require 'drupal/admin_toolbar:^3.5'
#  drush --yes pm:install neticrm_dmenu

  # add permission for unit testing
  drush role-add-perm anonymous 'profile create,register for events,access CiviMail subscribe/unsubscribe pages,access all custom data,view event info,view public CiviMail content,make online contributions'
  drush role-add-perm authenticated 'profile create,register for events,access CiviMail subscribe/unsubscribe pages,access all custom data,view event info,view public CiviMail content,make online contributions,profile edit'

  # add user login block to front page
  mkdir /tmp/config
  printf "langcode: en\nstatus: true\ndependencies:\n  module:\n    - user\n  theme:\n    - olivero\nid: userlogin\ntheme: olivero\nregion: sidebar\nweight: 0\nprovider: null\nplugin: user_login_block\nsettings:\n  id: user_login_block\n  label: 'User login'\n  label_display: visible\n  provider: user\nvisibility: {  }" > /tmp/config/block.block.userlogin.yml
  printf "langcode: en\nstatus: true\ndependencies:\n  module:\n    - image\n    - user\nid: user.user.default\ntargetEntityType: user\nbundle: user\nmode: default\ncontent:\n  member_for:\n    settings: {  }\n    third_party_settings: {  }\n    weight: 1\n    region: content\n  user_picture:\n    type: image\n    label: hidden\n    settings:\n      image_link: content\n      image_style: thumbnail\n      image_loading:\n        attribute: lazy\n    third_party_settings: {  }\n    weight: 0\n    region: content\n  civicrm_dashboard:\n    settings: {  }\n    third_party_settings: {  }\n    weight: 3\n    region: content\n  civicrm_profiles:\n    settings: {  }\n    third_party_settings: {  }\n    weight: 4\n    region: content\n  civicrm_record:\n    settings: {  }\n    third_party_settings: {  }\n    weight: 2\n    region: content\nhidden:\n\n" > /tmp/config/core.entity_view_display.user.user.default.yml
  drush --yes config:import --source=/tmp/config --partial

  chown -R www-data /var/www/html/sites/default/files
fi
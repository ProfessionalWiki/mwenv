# syntax=docker/dockerfile:1
ARG PHP_VERSION
FROM docker.io/library/php:${PHP_VERSION}-apache

RUN apt-get update \
	&& apt-get install -y --no-install-recommends git unzip \
	&& rm -rf /var/lib/apt/lists/*

COPY --from=docker.io/mlocati/php-extension-installer:2 /usr/bin/install-php-extensions /usr/local/bin/
RUN install-php-extensions apcu calendar gd intl mysqli opcache zip

COPY --from=docker.io/library/composer:2 /usr/bin/composer /usr/local/bin/composer

RUN a2enmod rewrite \
	&& a2disconf other-vhosts-access-log \
	&& git config --system safe.directory '*' \
	&& mkdir -m 1777 /composer-cache /var/www/data

ENV COMPOSER_CACHE_DIR=/composer-cache \
	COMPOSER_HOME=/tmp/composer \
	COMPOSER_ALLOW_SUPERUSER=1 \
	HOME=/tmp

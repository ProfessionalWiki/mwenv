# syntax=docker/dockerfile:1
ARG BASE_IMAGE
FROM ${BASE_IMAGE}

ARG CORE_REF
ARG VECTOR_REF

WORKDIR /var/www/html/w

# One layer, so that making every file writable does not copy the whole tree into a second layer.
RUN --mount=type=cache,target=/composer-cache \
	git clone --quiet --depth 1 --branch "$CORE_REF" https://gerrit.wikimedia.org/r/mediawiki/core . \
	&& find extensions skins -mindepth 1 -maxdepth 1 -type d -empty -delete \
	&& rm -rf skins/Vector \
	&& git clone --quiet --depth 1 --branch "$VECTOR_REF" https://gerrit.wikimedia.org/r/mediawiki/skins/Vector skins/Vector \
	&& composer install --no-interaction --no-progress \
	&& rm -rf "$COMPOSER_HOME" \
	&& chmod -R a+rwX /var/www/html

# Last, so that changing them does not rebuild the layer above.
COPY apache.conf /etc/apache2/sites-available/000-default.conf
COPY php.ini /usr/local/etc/php/conf.d/mwenv.ini

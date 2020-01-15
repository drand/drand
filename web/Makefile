SRC=public/
DEST=127.0.0.1:/var/www/html/

server:
	hugo server --buildDrafts --forceSyncStatic --disableFastRender --verbose

build:
	hugo

test-server:
	hugo --config config_test.toml
	hugo server --config config_test.toml --buildDrafts --forceSyncStatic --disableFastRender --verbose

deploy:
	rsync -Paivz --delete $(SRC) $(DEST)

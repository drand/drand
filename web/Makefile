SRC=public/
DEST=127.0.0.1:/var/www/html/

server:
	hugo server --buildDrafts --forceSyncStatic --disableFastRender --verbose

build:
	hugo

deploy:
	rsync -Paivz --delete $(SRC) $(DEST)

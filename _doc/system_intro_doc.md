# SeeDAO App Backend Deploy Documents

SeeDAO App backend contains those components:

* OS Backend API Service
* Database
* SNS Profile Service
* Indexer Service
* SNS Safe Check Service 

This document will describe deployments for those components.

## Deployment for Various Environments

Currently, there are 3 environments for SeeDAO apps, which are:

* dev: https://dev-app.seedao.tech/
* preview: https://preview-app.seedao.tech/
* prod: https://app.seedao.xyz/

Environment dev and preview are deployed on same node, while prod env are deployed in another node.
The detail of service will be described in deployment section.

### Dev Deployment

Currently, the dev deployment contains os-backend and spp-indexer.

os-backend in dev environment is deployed via pm2, and the detail can be found in each component

### Preview Deployment

Preview services are launched via docker compose on dev node.
Remember to migrate DB if there are any database related changes, and also remember to backup all data before migration.

```shell
# Update images
docker pull ghcr.io/taoist-labs/os-backend:preview
docker pull ghcr.io/taoist-labs/os-push-backend:main
docker pull ghcr.io/taoist-labs/spp-profile-backend:master
docker pull ghcr.io/taoist-labs/sns-safe-backend:main
docker pull ghcr.io/taoist-labs/spp-indexer:dev
docker pull ghcr.io/taoist-labs/spp-indexer:preview
docker pull ghcr.io/taoist-labs/spp-indexer:main
docker pull ghcr.io/taoist-labs/spp-chain-data-crawler:master

cd /home/ubuntu/seedao/compose
docker compose up -d
```

### Production Deployment

Production is also deployed via docker compose. Notice that the image tag pulled is different with preview environment.

```shell
ssh <prod host>

docker pull ghcr.io/taoist-labs/os-backend:main
docker pull ghcr.io/taoist-labs/spp-profile-backend:master
docker pull ghcr.io/taoist-labs/sns-safe-backend:main
docker pull ghcr.io/taoist-labs/spp-indexer:main
docker pull ghcr.io/taoist-labs/spp-chain-data-crawler:master

cd /srv/seedao
docker compose up -d
```

---

## SeeDAO OS Backend

Main api service for SeeDAO App.

Code repo: [https://github.com/TheSeed-Labs/os-backend](https://github.com/TheSeed-Labs/os-backend)

### Test:

```shell
go test ./...
```

This test is integrated in GitHub CI process,
but the case can't cover all code since some logics are based on 3rd part services which can't be tested automatically.

### General Update Process

1. Backup database
2. Migrate database with dbutils built by **LATEST CODE**.
3. Review database changes
4. Restart API service

### Configuration

Launching this service requires two files, which are configuration file and casbin config file *rbac_model.conf*.
The casbin config file is in the repo that can be used directly, the configuration file should be updated based on current running env.

### Environments

OS backend service has three environments, which are dev, preview and prod.
dev and preview env are launched on same node, while prod is in separated node.

Here only lists the deployment on dev environment since the preview and production are docker compose version and is integrated with other services. 

If there are existing API service running:

```shell
ssh <dev node>
cd /home/ubuntu/seedao/dev-os-backend
./update_binary.sh
```

Here is the content of script:

```shell
#!/bin/bash

set -x

pushd src/
git branch
git pull -r
make build
pm2 stop 1
cp bin/apiserver ../
pm2 start 1
popd
```

This script get latest code from GitHub and build binaries, then restart the service launched by pm2.

**NOTE**: The index number in `pm2 start` command may be changed after host restarted or service recreated.


If this is the first time to launch the service (no pm2 entry created), then run this command:

```shell
pm2 start ./apiserver
```


### Utils

The project contains the db utils which can migrate table schema.
Put the configuration file with DSN section with working directory and launch the binary.
If no error occurred, the migrations has finished.

Here is the command used for migrating table changes:

```shell
./bin/dbutils migrate  -migrate-table
```

Another utils `records_loader` is used to load applications in Excel file to SeeDAO OS database.

```bash
$ ./records_loader -dsn <dsn> -season <season name> -detail-sheet <sheet name> -input <input_file>
```

---

## Spp Backend Service

Response to seepass data querying.

### General Update Process

1. Backup database
2. Migrate database with prisma
3. Restart API service

### Environments

The service only has two running instances, which are dev/preview and production, and both are deployed by docker compose.

_Dev_:

```shell
docker pull ghcr.io/taoist-labs/os-backend
cd /home/ubuntu/seedao/compose
docker pull ghcr.io/taoist-labs/spp-profile-backend:master
```

If there are db change, launch the migration instance manually:


```shell
docker pull ghcr.io/taoist-labs/os-backend
cd /home/ubuntu/seedao/compose
docker pull ghcr.io/taoist-labs/spp-profile-backend:master
```

---

## Database

Database is used to storing SeeDAO OS app data and indexer data.
The database service currently used is PostgreSQL 16.

The database used for services are configured by config.yml file.

### Schema Name

The database is running as normal service on dev/preview and prod environment.
Here are schema name of three services using this DB for saving data.

_Dev_

* os-backend: os_backend_dev
* spp_indexer: spp_indexer_test

_Preview_:

* os-backend: os_backend_preview
* spp_indexer: spp_indexer_preview
* spp_api: spp_test

_Prod_:

* os-backend: os_backend_prod
* spp_indexer: spp_indexer_prod
* spp_api: spp_prod


### Migrate

**NOTE**: DO FULLY BACKUP BEFORE CHANGING ANY DB SCHEMAS

#### SeeDAO OS App

SeeDAO OS app has various models which are mapping to database schemas via gorm framework.
Creating and migrating schemas requires the *dbutils* tool mentioned above.
It can handle migration tasks automatically and without any output.
So if there are any output shown while running the command, please check it carefully.

Here is the command to migrate SeeDAO OS app schema, there should be a *config.yaml* file in the running directory with correct DSN setup.

```shell
./bin/dbutils migrate  -migrate-table
```

#### Spp Indexer and Spp Profile API

Those two services are written in Typescript and using Prisma as DB ORM.
Before migrating the schemas, the migration file should be commit to repo and downloaded to working copy.
Then execute the command below to migrate DB

```shell
yarn prisma migrate dev
```

The DB is configured via *envfile* under running directory.

### Backup

Database in production environment currently is backup by cronjob every 4 hours. Here is the content of backup scripts:

```shell
#!/bin/bash

# Set the database name and S3 bucket name
DB_USER="os_backend_prod_user"
DB_PASS="luvk3Ttsn6x21NU"
S3_BUCKET="backup-seedao-prod-db"

export PGPASSWORD=${DB_PASS}

DB_LIST=("os_backend_prod spp_prod spp_indexer_prod")
TS=$(date +%Y%m%d-%H%M)

for DB in ${DB_LIST[@]}; do
  backup_filename=$DB.$TS.pgdump
  sudo -u postgres pg_dump -Fc -c ${DB} >${backup_filename}
  aws s3 cp ${backup_filename} s3://$S3_BUCKET/${backup_filename}
  rm ${backup_filename}
done
```

The script uses *pg_dump* to dump all data from database table and upload to S3 bucket.

---

## Spp Profile API

This service provides API of accessing SNS ID, which is developed via Typescript.
The source code can be found at [https://github.com/Taoist-Labs/spp-profile-backend](https://github.com/Taoist-Labs/spp-profile-backend).

### General Update Process

1. Update code
2. Migrate database if required
3. Restart the application

---

## Spp Indexer Service

This service indexing smart contract's events and provide api for query data.
The source code can be found at [https://github.com/Taoist-Labs/spp-indexer](https://github.com/Taoist-Labs/spp-indexer)

### General Update Process

1. Update code
2. Migrate database if required
3. Restart the application

---

## SNS Safe Check Service

This service is for checking if a SNS name is sensitive or reserved. 
The source code can be found at [https://github.com/Taoist-Labs/sns-safe-backend](https://github.com/Taoist-Labs/sns-safe-backend)

### General Update Process

1. Update code
2. Migrate database if required
3. Restart the application


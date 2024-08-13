# SeeDAO App Backend Deploy Documents

SeeDAO App backend contains those components:

* OS Backend API Service
* Database
* SNS Profile Service
* Indexer Service
* SNS Safe Check Service 

This document will describe deployments for those components.

## Environment

Currently, there are 3 environments for SeeDAO apps, which are:

* dev: https://dev-app.seedao.tech/
* preview: https://preview-app.seedao.tech/
* prod: https://app.seedao.xyz/

Environment dev and preview are deployed on same node, while prod env are deployed in another node.
The detail of service will be described in deployment section.

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

_Dev_:

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

**NOTE**: The index number in `pm2 start` command may be changed after restart.


If this is the first time to launch the service (no pm2 entry created), then run this command:

```shell
pm2 start ./apiserver
```

_Preview_:

Preview is deployed via docker container, the GitHub ci will build the docker image `ghcr.io/taoist-labs/os-backend:preview` if no errors found.

> Migrating database is same with last step.

```shell
docker pull ghcr.io/taoist-labs/os-backend
cd /home/ubuntu/seedao/compose
docker compose up -d os-backend:dev
```

_Production_:

Production is also deployed by docker.

```shell
ssh <prod host>
cd /srv/seedao
# Update images
docker compose up -d
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

#### Migrate

**NOTE**: DO FULLY BACKUP BEFORE CHANGING ANY DB SCHEMAS

SeeDAO OS app has various models which are mapping to database schemas via gorm framework.
Creating and migrating schemas requires the *dbutils* tool mentioned above.
It can handle migration tasks automatically and without any output.
So if there are any output shown while running the command, please check it carefully.

#### Backup

Database currently is backup by cronjob every 4 hours. Here is the content of backup scripts:

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
---
---
---
---
---

### SNS Profile Service

This service provides API of accessing SNS ID, which is developed via Typescript.
The source code can be found [here](https://github.com/Taoist-Labs/spp-profile-backend).

## Dev and Preview Deployment

Currently only 


## Prod Deployment


#!/bin/sh -l

cd /opt/www/

echo 'Building the UpYun-Deployer...'

go build .

echo 'Deploying to UpYun...'

./upyun-deployer -bucket "$INPUT_BUCKET" -operator "$INPUT_OPERATOR" -password "$INPUT_PASSWORD" -local_dir "$GITHUB_WORKSPACE/$INPUT_DIR" -publish_dir "$INPUT_PUBLISH_DIR"

echo 'Complete'

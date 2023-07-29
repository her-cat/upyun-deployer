#!/bin/sh -l

cd /opt/www/

echo 'Building the UpYun-Deployer...'

go build .

echo 'Deploying to UpYun...'

./upyun-deployer -bucket "$INPUT_BUCKET" -operator "$INPUT_OPERATOR" -password "$INPUT_PASSWORD" -dir "$GITHUB_WORKSPACE/$INPUT_DIR"

echo 'Complete'

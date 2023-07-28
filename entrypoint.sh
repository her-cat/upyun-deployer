#!/bin/sh -l

echo "bucket: $INPUT_BUCKET operator: $INPUT_OPERATOR password: $INPUT_PASSWORD dir: $INPUT_DIR"

cd /opt/www/

echo 'Building the UpYun-Deployer...'

go build .

echo 'Deploying to UpYun...'

./upyun-deployer -bucket "$INPUT_BUCKET" -operator "$INPUT_OPERATOR" -password "$INPUT_PASSWORD" -dir "$INPUT_DIR"

echo 'Complete'

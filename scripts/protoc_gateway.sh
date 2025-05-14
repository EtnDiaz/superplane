#!/bin/bash

<< 'DOCS'
  Generate gRPC-Gateway and OpenAPI specs from internal_api definitions.
  This script uses the existing HTTP annotations in the proto file.
DOCS

# When DEBUG env is set - print output of the scripts.
if [[ -e $DEBUG ]];
then
  set -x
fi

INTERNAL_API_REPO=git@github.com:renderedtext/internal_api.git
INTERNAL_API_OUT=pkg/protos
GATEWAY_OUT=pkg/protos
OPENAPI_OUT=api/swagger
MODULE_NAME=github.com/superplanehq/superplane
MODULES=(${1//,/ })
INTERNAL_API_BRANCH=${2:-master}
INTERNAL_API_FOLDER=${3:-"/tmp/internal_api"}

# Install required third-party proto files if not already present
install_third_party_protos() {
  THIRD_PARTY_DIR=$INTERNAL_API_FOLDER/include/google/api
  if [ ! -d "$THIRD_PARTY_DIR" ]; then
    echo "$(bold "Installing Google API proto files")"
    mkdir -p $THIRD_PARTY_DIR
    curl -L https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/annotations.proto > $THIRD_PARTY_DIR/annotations.proto
    curl -L https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/http.proto > $THIRD_PARTY_DIR/http.proto
    curl -L https://raw.githubusercontent.com/googleapis/googleapis/master/google/api/field_behavior.proto > $THIRD_PARTY_DIR/field_behavior.proto
  fi

  OPENAPI_DIR=$INTERNAL_API_FOLDER/include/protoc-gen-openapiv2/options
  if [ ! -d "$OPENAPI_DIR" ]; then
    echo "$(bold "Installing OpenAPI proto files")"
    mkdir -p $OPENAPI_DIR
    curl -L https://raw.githubusercontent.com/grpc-ecosystem/grpc-gateway/main/protoc-gen-openapiv2/options/annotations.proto > $OPENAPI_DIR/annotations.proto
    curl -L https://raw.githubusercontent.com/grpc-ecosystem/grpc-gateway/main/protoc-gen-openapiv2/options/openapiv2.proto > $OPENAPI_DIR/openapiv2.proto
  fi
}

# Check if the proto file already has the necessary imports
check_and_add_imports() {
  MODULE=$1
  FILE=$2

  echo "$(bold "Checking and adding required imports to $FILE")"

  # Check if annotations import exists, if not, add it
  if ! grep -q "import \"google/api/annotations.proto\";" "$FILE"; then
    sed -i '/import "google\/protobuf\/timestamp.proto";/a import "google\/api\/annotations.proto";' "$FILE"
    echo "Added Google API annotations import"
  else
    echo "Google API annotations import already exists"
  fi

  # Check if OpenAPI import exists, if not, add it
  if ! grep -q "import \"protoc-gen-openapiv2/options/annotations.proto\";" "$FILE"; then
    sed -i '/import "google\/api\/annotations.proto";/a import "protoc-gen-openapiv2\/options\/annotations.proto";' "$FILE"
    echo "Added OpenAPI annotations import"
  else
    echo "OpenAPI annotations import already exists"
  fi
}

generate_gateway_files() {
  MODULE=$1
  FILE=$2

  echo "$(bold "Generating gRPC-Gateway files for $MODULE")"
  
  # Create output directories
  mkdir -p $GATEWAY_OUT/$MODULE
  mkdir -p $OPENAPI_OUT

  # Generate gRPC-Gateway code
  protoc --proto_path $INTERNAL_API_FOLDER/ \
         --proto_path $INTERNAL_API_FOLDER/include \
         --proto_path $INTERNAL_API_FOLDER/include/internal_api \
         --grpc-gateway_out=$GATEWAY_OUT/$MODULE \
         --grpc-gateway_opt=logtostderr=true \
         --grpc-gateway_opt=paths=source_relative \
         --openapiv2_out=$OPENAPI_OUT \
         --openapiv2_opt=logtostderr=true \
         $FILE
         
  echo "Generated gRPC-Gateway files in $GATEWAY_OUT/$MODULE"
  echo "Generated OpenAPI specification in $OPENAPI_OUT"
}

bold() {
  bold_text=$(tput bold)
  normal_text=$(tput sgr0)
  echo -n "${bold_text}$@${normal_text}"
}

# Main execution
for MODULE in ${MODULES[@]};
do
  install_third_party_protos
  check_and_add_imports $MODULE $INTERNAL_API_FOLDER/$MODULE.proto
  generate_gateway_files $MODULE $INTERNAL_API_FOLDER/$MODULE.proto
done

echo "$(bold "Done generating gRPC-Gateway for: ${MODULES[@]}")"
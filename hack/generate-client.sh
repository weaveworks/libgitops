#!/bin/bash

SCRIPT_DIR=$( dirname "${BASH_SOURCE[0]}" )
cd ${SCRIPT_DIR}/..

RESOURCES="Car Motorcycle"
CLIENT_NAME=SampleInternal
OUT_DIR=cmd/sample-app/client
API_DIR="github.com/weaveworks/libgitops/cmd/sample-app/apis/sample"
mkdir -p ${OUT_DIR}
for Resource in ${RESOURCES}; do
    resource=$(echo "${Resource}" | awk '{print tolower($0)}')
    sed -e "s|Resource|${Resource}|g;s|resource|${resource}|g;/build ignore/d;s|API_DIR|${API_DIR}|g;s|*Client|*${CLIENT_NAME}Client|g" \
        pkg/client/client_resource_template.go > \
        ${OUT_DIR}/zz_generated.client_${resource}.go
done

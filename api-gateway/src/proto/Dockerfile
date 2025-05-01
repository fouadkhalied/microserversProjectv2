FROM grpc/grpc:latest

WORKDIR /proto
COPY . .

RUN protoc --js_out=import_style=commonjs,binary:./proto --grpc_out=grpc_js:./proto --proto_path=proto proto/user.proto

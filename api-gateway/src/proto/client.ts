// client.ts
import grpc from '@grpc/grpc-js';
import protoLoader from '@grpc/proto-loader';

const packageDef = protoLoader.loadSync('../proto/user.proto');
const grpcObj = grpc.loadPackageDefinition(packageDef) as any;
const clientGRPC = new grpcObj.UserService('localhost:50051', grpc.credentials.createInsecure());

export default clientGRPC;

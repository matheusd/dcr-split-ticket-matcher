// GENERATED CODE -- DO NOT EDIT!

'use strict';
var grpc = require('grpc');
var api_pb = require('./api_pb.js');

function serialize_dcrticketmatcher_BuyerErrorRequest(arg) {
  if (!(arg instanceof api_pb.BuyerErrorRequest)) {
    throw new Error('Expected argument of type dcrticketmatcher.BuyerErrorRequest');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_BuyerErrorRequest(buffer_arg) {
  return api_pb.BuyerErrorRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_BuyerErrorResponse(arg) {
  if (!(arg instanceof api_pb.BuyerErrorResponse)) {
    throw new Error('Expected argument of type dcrticketmatcher.BuyerErrorResponse');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_BuyerErrorResponse(buffer_arg) {
  return api_pb.BuyerErrorResponse.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_FindMatchesRequest(arg) {
  if (!(arg instanceof api_pb.FindMatchesRequest)) {
    throw new Error('Expected argument of type dcrticketmatcher.FindMatchesRequest');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_FindMatchesRequest(buffer_arg) {
  return api_pb.FindMatchesRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_FindMatchesResponse(arg) {
  if (!(arg instanceof api_pb.FindMatchesResponse)) {
    throw new Error('Expected argument of type dcrticketmatcher.FindMatchesResponse');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_FindMatchesResponse(buffer_arg) {
  return api_pb.FindMatchesResponse.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_FundSplitTxRequest(arg) {
  if (!(arg instanceof api_pb.FundSplitTxRequest)) {
    throw new Error('Expected argument of type dcrticketmatcher.FundSplitTxRequest');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_FundSplitTxRequest(buffer_arg) {
  return api_pb.FundSplitTxRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_FundSplitTxResponse(arg) {
  if (!(arg instanceof api_pb.FundSplitTxResponse)) {
    throw new Error('Expected argument of type dcrticketmatcher.FundSplitTxResponse');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_FundSplitTxResponse(buffer_arg) {
  return api_pb.FundSplitTxResponse.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_FundTicketRequest(arg) {
  if (!(arg instanceof api_pb.FundTicketRequest)) {
    throw new Error('Expected argument of type dcrticketmatcher.FundTicketRequest');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_FundTicketRequest(buffer_arg) {
  return api_pb.FundTicketRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_FundTicketResponse(arg) {
  if (!(arg instanceof api_pb.FundTicketResponse)) {
    throw new Error('Expected argument of type dcrticketmatcher.FundTicketResponse');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_FundTicketResponse(buffer_arg) {
  return api_pb.FundTicketResponse.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_GenerateTicketRequest(arg) {
  if (!(arg instanceof api_pb.GenerateTicketRequest)) {
    throw new Error('Expected argument of type dcrticketmatcher.GenerateTicketRequest');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_GenerateTicketRequest(buffer_arg) {
  return api_pb.GenerateTicketRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_GenerateTicketResponse(arg) {
  if (!(arg instanceof api_pb.GenerateTicketResponse)) {
    throw new Error('Expected argument of type dcrticketmatcher.GenerateTicketResponse');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_GenerateTicketResponse(buffer_arg) {
  return api_pb.GenerateTicketResponse.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_StatusRequest(arg) {
  if (!(arg instanceof api_pb.StatusRequest)) {
    throw new Error('Expected argument of type dcrticketmatcher.StatusRequest');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_StatusRequest(buffer_arg) {
  return api_pb.StatusRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_StatusResponse(arg) {
  if (!(arg instanceof api_pb.StatusResponse)) {
    throw new Error('Expected argument of type dcrticketmatcher.StatusResponse');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_StatusResponse(buffer_arg) {
  return api_pb.StatusResponse.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_WatchWaitingListRequest(arg) {
  if (!(arg instanceof api_pb.WatchWaitingListRequest)) {
    throw new Error('Expected argument of type dcrticketmatcher.WatchWaitingListRequest');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_WatchWaitingListRequest(buffer_arg) {
  return api_pb.WatchWaitingListRequest.deserializeBinary(new Uint8Array(buffer_arg));
}

function serialize_dcrticketmatcher_WatchWaitingListResponse(arg) {
  if (!(arg instanceof api_pb.WatchWaitingListResponse)) {
    throw new Error('Expected argument of type dcrticketmatcher.WatchWaitingListResponse');
  }
  return new Buffer(arg.serializeBinary());
}

function deserialize_dcrticketmatcher_WatchWaitingListResponse(buffer_arg) {
  return api_pb.WatchWaitingListResponse.deserializeBinary(new Uint8Array(buffer_arg));
}


var SplitTicketMatcherServiceService = exports.SplitTicketMatcherServiceService = {
  watchWaitingList: {
    path: '/dcrticketmatcher.SplitTicketMatcherService/WatchWaitingList',
    requestStream: false,
    responseStream: true,
    requestType: api_pb.WatchWaitingListRequest,
    responseType: api_pb.WatchWaitingListResponse,
    requestSerialize: serialize_dcrticketmatcher_WatchWaitingListRequest,
    requestDeserialize: deserialize_dcrticketmatcher_WatchWaitingListRequest,
    responseSerialize: serialize_dcrticketmatcher_WatchWaitingListResponse,
    responseDeserialize: deserialize_dcrticketmatcher_WatchWaitingListResponse,
  },
  findMatches: {
    path: '/dcrticketmatcher.SplitTicketMatcherService/FindMatches',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.FindMatchesRequest,
    responseType: api_pb.FindMatchesResponse,
    requestSerialize: serialize_dcrticketmatcher_FindMatchesRequest,
    requestDeserialize: deserialize_dcrticketmatcher_FindMatchesRequest,
    responseSerialize: serialize_dcrticketmatcher_FindMatchesResponse,
    responseDeserialize: deserialize_dcrticketmatcher_FindMatchesResponse,
  },
  generateTicket: {
    path: '/dcrticketmatcher.SplitTicketMatcherService/GenerateTicket',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.GenerateTicketRequest,
    responseType: api_pb.GenerateTicketResponse,
    requestSerialize: serialize_dcrticketmatcher_GenerateTicketRequest,
    requestDeserialize: deserialize_dcrticketmatcher_GenerateTicketRequest,
    responseSerialize: serialize_dcrticketmatcher_GenerateTicketResponse,
    responseDeserialize: deserialize_dcrticketmatcher_GenerateTicketResponse,
  },
  fundTicket: {
    path: '/dcrticketmatcher.SplitTicketMatcherService/FundTicket',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.FundTicketRequest,
    responseType: api_pb.FundTicketResponse,
    requestSerialize: serialize_dcrticketmatcher_FundTicketRequest,
    requestDeserialize: deserialize_dcrticketmatcher_FundTicketRequest,
    responseSerialize: serialize_dcrticketmatcher_FundTicketResponse,
    responseDeserialize: deserialize_dcrticketmatcher_FundTicketResponse,
  },
  fundSplitTx: {
    path: '/dcrticketmatcher.SplitTicketMatcherService/FundSplitTx',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.FundSplitTxRequest,
    responseType: api_pb.FundSplitTxResponse,
    requestSerialize: serialize_dcrticketmatcher_FundSplitTxRequest,
    requestDeserialize: deserialize_dcrticketmatcher_FundSplitTxRequest,
    responseSerialize: serialize_dcrticketmatcher_FundSplitTxResponse,
    responseDeserialize: deserialize_dcrticketmatcher_FundSplitTxResponse,
  },
  status: {
    path: '/dcrticketmatcher.SplitTicketMatcherService/Status',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.StatusRequest,
    responseType: api_pb.StatusResponse,
    requestSerialize: serialize_dcrticketmatcher_StatusRequest,
    requestDeserialize: deserialize_dcrticketmatcher_StatusRequest,
    responseSerialize: serialize_dcrticketmatcher_StatusResponse,
    responseDeserialize: deserialize_dcrticketmatcher_StatusResponse,
  },
  buyerError: {
    path: '/dcrticketmatcher.SplitTicketMatcherService/BuyerError',
    requestStream: false,
    responseStream: false,
    requestType: api_pb.BuyerErrorRequest,
    responseType: api_pb.BuyerErrorResponse,
    requestSerialize: serialize_dcrticketmatcher_BuyerErrorRequest,
    requestDeserialize: deserialize_dcrticketmatcher_BuyerErrorRequest,
    responseSerialize: serialize_dcrticketmatcher_BuyerErrorResponse,
    responseDeserialize: deserialize_dcrticketmatcher_BuyerErrorResponse,
  },
};

exports.SplitTicketMatcherServiceClient = grpc.makeGenericClientConstructor(SplitTicketMatcherServiceService);

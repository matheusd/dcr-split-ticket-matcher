#!/bin/sh

protoc -I. api.proto --go_out=plugins=grpc:matcherrpc
protoc -I. integrator-api.proto --go_out=plugins=grpc:integratorrpc

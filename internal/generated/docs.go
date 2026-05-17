//go:generate mkdir -p api/chat
//go:generate protoc --proto_path=../../api --go_out=api/chat --go_opt=paths=source_relative message.proto

package generated

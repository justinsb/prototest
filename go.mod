module prototest

go 1.15

require (
	github.com/gogo/protobuf v1.3.1
	github.com/golang/protobuf v1.4.3
	github.com/google/go-cmp v0.5.0
	go.etcd.io/etcd/api/v3 v3.0.0-20210107172604-c632042bb96c // indirect
	go.etcd.io/etcd/pkg/v3 v3.5.0-pre // indirect
	go.etcd.io/etcd/raft/v3 v3.0.0-20210107172604-c632042bb96c
	google.golang.org/protobuf v1.25.0
	k8s.io/klog v1.0.0
)

replace go.etcd.io/etcd/pkg/v3 => go.etcd.io/etcd/pkg/v3 v3.0.0-20210107172604-c632042bb96c

replace google.golang.org/protobuf => ./forks/github.com/protocolbuffers/protobuf-go
replace github.com/gogo/protobuf => ./forks/github.com/gogo/protobuf
replace github.com/golang/protobuf => ./forks/github.com/golang/protobuf
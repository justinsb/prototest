package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"

	// gogodescriptor "github.com/gogo/protobuf/protoc-gen-gogo/descriptor"
	gogoproto "github.com/gogo/protobuf/proto"
	oldproto "github.com/golang/protobuf/proto"
	"github.com/google/go-cmp/cmp"
	"go.etcd.io/etcd/raft/v3/raftpb"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	newproto "google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/runtime/protoiface"
	"google.golang.org/protobuf/types/descriptorpb"
	"k8s.io/klog"
)

func main() {
	err := run(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
	}
}

func checkNewProto(in newproto.Message, out newproto.Message) error {
	msg, err := newproto.Marshal(in)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	hasher := sha256.New()
	hasher.Write(msg)
	hash := hasher.Sum(nil)

	fmt.Printf("%x\n", hash)

	if err := newproto.Unmarshal(msg, out); err != nil {
		return fmt.Errorf("failed to unmarshal: %w", err)
	}

	s := prototext.MarshalOptions{Multiline: false}.Format(in)
	fmt.Printf("%s\n", s)
	return nil
}

func checkOldProto(in oldproto.Message, out oldproto.Message) error {
	msg, err := oldproto.Marshal(in)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	hasher := sha256.New()
	hasher.Write(msg)
	hash := hasher.Sum(nil)

	fmt.Printf("%x\n", hash)

	if err := oldproto.Unmarshal(msg, out); err != nil {
		return fmt.Errorf("failed to unmarshal: %w", err)
	}

	s := in.String()
	fmt.Printf("%s\n", s)
	return nil
}

// func run1(ctx context.Context) error {
// 	in := gogo.Person{
// 		Id:    1234,
// 		Name:  "John Doe",
// 		Email: "jdoe@example.com",
// 		Phones: []*gogo.Person_PhoneNumber{
// 			{Number: "555-4321", Type: gogo.Person_HOME},
// 		},
// 	}

// 	out := gogo.Person{}

// 	return check(&in, &out)
// }

// func run2(ctx context.Context) error {
// 	in := etcdserverpb.PutRequest{
// 		Key:   []byte("foo"),
// 		Value: []byte("value"),
// 	}

// 	out := etcdserverpb.PutRequest{}

// 	return check(&in, &out)
// }

func run(ctx context.Context) error {
	in := raftpb.Message{
		Type: raftpb.MsgBeat,
	}

	out := raftpb.Message{}

	if err := checkOldProto(&in, &out); err != nil {
		return err
	}

	if diff := cmp.Diff(in, out); diff != "" {
		return fmt.Errorf("got a diff: %v", diff)
	}

	if err := checkNewProto(&adapter{msg: &in}, &adapter{msg: &out}); err != nil {
		return err
	}

	if diff := cmp.Diff(in, out); diff != "" {
		return fmt.Errorf("got a diff: %v", diff)
	}

	if err := checkNewProto(oldproto.MessageV2(&in), oldproto.MessageV2(&out)); err != nil {
		return err
	}

	if diff := cmp.Diff(in, out); diff != "" {
		return fmt.Errorf("got a diff: %v", diff)
	}

	return nil
}

type gogoMessage interface {
	Descriptor() (gz []byte, path []int)
	Size() int
	Reset()
	Unmarshal([]byte) error
	MarshalToSizedBuffer([]byte) (int, error)
}

type adapter struct {
	msg     gogoMessage
	msgDesc protoreflect.MessageDescriptor
}

func (a *adapter) init() error {
	fd, md := /*gogodescriptor.*/ forMessage(a.msg.Descriptor())

	var resolver protodesc.Resolver
	rfd, err := protodesc.FileOptions{AllowUnresolvable: true}.New(fd, resolver)
	if err != nil {
		return fmt.Errorf("cannot parse FileDescriptor: %v", err)
	}
	messageName := protoreflect.Name(md.GetName())
	msgDesc := rfd.Messages().ByName(messageName)
	if msgDesc == nil {
		return fmt.Errorf("cannot get message %q: %v", messageName, err)
	}
	a.msgDesc = msgDesc
	return nil
}

func (a *adapter) ProtoReflect() protoreflect.Message {
	// TODO: Can this be shared across objects?
	return a
}

// extractFile extracts a FileDescriptorProto from a gzip'd buffer.
func extractFile(gz []byte) (*descriptorpb.FileDescriptorProto, error) {
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip reader: %v", err)
	}
	defer r.Close()

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to uncompress descriptor: %v", err)
	}

	fd := new(descriptorpb.FileDescriptorProto)
	if err := proto.Unmarshal(b, fd); err != nil {
		return nil, fmt.Errorf("malformed FileDescriptorProto: %v", err)
	}

	return fd, nil
}

// forMessage returns a FileDescriptorProto and a DescriptorProto from within it
// describing the given message.
func forMessage(gz []byte, path []int) (fd *descriptorpb.FileDescriptorProto, md *descriptorpb.DescriptorProto) {
	// gz, path := msg.Descriptor()
	fd, err := extractFile(gz)
	if err != nil {
		panic(fmt.Sprintf("invalid FileDescriptorProto: %v", err))
	}

	md = fd.MessageType[path[0]]
	for _, i := range path[1:] {
		md = md.NestedType[i]
	}
	return fd, md
}

// Descriptor returns message descriptor, which contains only the protobuf
// type information for the message.
func (a *adapter) Descriptor() protoreflect.MessageDescriptor {
	if a.msgDesc == nil {
		if err := a.init(); err != nil {
			klog.Fatalf("init failed: %v", err)
		}
	}
	return a.msgDesc
}

// Type returns the message type, which encapsulates both Go and protobuf
// type information. If the Go type information is not needed,
// it is recommended that the message descriptor be used instead.
func (a *adapter) Type() protoreflect.MessageType {
	klog.Fatalf("not implemented")
	return nil
}

// New returns a newly allocated and mutable empty message.
func (a *adapter) New() protoreflect.Message {
	klog.Fatalf("not implemented")
	return nil

}

// Interface unwraps the message reflection interface and
// returns the underlying ProtoMessage interface.
func (a *adapter) Interface() protoreflect.ProtoMessage {
	return a
}

// Range iterates over every populated field in an undefined order,
// calling f for each field descriptor and value encountered.
// Range returns immediately if f returns false.
// While iterating, mutating operations may only be performed
// on the current field descriptor.
func (a *adapter) Range(f func(protoreflect.FieldDescriptor, protoreflect.Value) bool) {
	v := reflect.ValueOf(a.msg)
	v = v.Elem()
	vt := v.Type()

	msgDesc := a.Descriptor()

	props := gogoproto.GetProperties(vt)
	// klog.Infof("props: %+v", props)
	for _, prop := range props.Prop {
		if prop.Tag == 0 {
			continue
		}

		fieldValue := v.FieldByName(prop.Name)

		fd := msgDesc.Fields().ByName(protoreflect.Name(prop.OrigName))
		if fd == nil {
			klog.Fatalf("cannot find proto field %q: %+v", prop.OrigName, prop)
		}
		var val protoreflect.Value
		if fd.IsList() {
			val = protoreflect.ValueOfList(&listAdapter{fieldValue})
		} else {
			switch fd.Kind() {
			case protoreflect.EnumKind:
				val = protoreflect.ValueOfEnum(protoreflect.EnumNumber(fieldValue.Int()))
			case protoreflect.Uint64Kind:
				val = protoreflect.ValueOfUint64(fieldValue.Uint())
			case protoreflect.BoolKind:
				val = protoreflect.ValueOfBool(fieldValue.Bool())
			case protoreflect.BytesKind:
				val = protoreflect.ValueOfBytes(fieldValue.Bytes())
			case protoreflect.MessageKind:
				if fieldValue.Kind() != reflect.Ptr {
					fieldValue = fieldValue.Addr()
				}
				i := fieldValue.Interface()
				gogo, ok := i.(gogoMessage)
				if !ok {
					klog.Fatalf("message type %T does not implement gogoMessage", i)
				}
				val = protoreflect.ValueOfMessage(&adapter{msg: gogo})
			default:
				klog.Fatalf("unhandled protoField kind %q", fd.Kind())
			}
		}

		// klog.Infof("fieldValue: %s %+v", fd.Name(), val)
		if !f(fd, val) {
			break
		}

	}
	// for i := 0; i < vt.NumField(); i++ {
	// 	f := vt.Field(i)
	// 	klog.Infof("field %v", f.Name)
	// 	klog.Infof("  tag %v", f.Tag)
	// 	var props gogoproto.Properties
	// 	props.Init()
	// }
	// klog.Fatalf("Range not implemented")
}

// Has reports whether a field is populated.
//
// Some fields have the property of nullability where it is possible to
// distinguish between the default value of a field and whether the field
// was explicitly populated with the default value. Singular message fields,
// member fields of a oneof, and proto2 scalar fields are nullable. Such
// fields are populated only if explicitly set.
//
// In other cases (aside from the nullable cases above),
// a proto3 scalar field is populated if it contains a non-zero value, and
// a repeated field is populated if it is non-empty.
func (a *adapter) Has(protoreflect.FieldDescriptor) bool {
	panic("not implemented")
}

// Clear clears the field such that a subsequent Has call reports false.
//
// Clearing an extension field clears both the extension type and value
// associated with the given field number.
//
// Clear is a mutating operation and unsafe for concurrent use.
func (a *adapter) Clear(protoreflect.FieldDescriptor) {
	klog.Fatalf("not implemented")
}

// Get retrieves the value for a field.
//
// For unpopulated scalars, it returns the default value, where
// the default value of a bytes scalar is guaranteed to be a copy.
// For unpopulated composite types, it returns an empty, read-only view
// of the value; to obtain a mutable reference, use Mutable.
func (a *adapter) Get(protoreflect.FieldDescriptor) protoreflect.Value {
	klog.Fatalf("not implemented")
	return protoreflect.Value{}
}

// Set stores the value for a field.
//
// For a field belonging to a oneof, it implicitly clears any other field
// that may be currently set within the same oneof.
// For extension fields, it implicitly stores the provided ExtensionType.
// When setting a composite type, it is unspecified whether the stored value
// aliases the source's memory in any way. If the composite value is an
// empty, read-only value, then it panics.
//
// Set is a mutating operation and unsafe for concurrent use.
func (a *adapter) Set(protoreflect.FieldDescriptor, protoreflect.Value) {
	klog.Fatalf("not implemented")
}

// Mutable returns a mutable reference to a composite type.
//
// If the field is unpopulated, it may allocate a composite value.
// For a field belonging to a oneof, it implicitly clears any other field
// that may be currently set within the same oneof.
// For extension fields, it implicitly stores the provided ExtensionType
// if not already stored.
// It panics if the field does not contain a composite type.
//
// Mutable is a mutating operation and unsafe for concurrent use.
func (a *adapter) Mutable(protoreflect.FieldDescriptor) protoreflect.Value {
	klog.Fatalf("not implemented")
	return protoreflect.Value{}
}

// NewField returns a new value that is assignable to the field
// for the given descriptor. For scalars, this returns the default value.
// For lists, maps, and messages, this returns a new, empty, mutable value.
func (a *adapter) NewField(protoreflect.FieldDescriptor) protoreflect.Value {
	klog.Fatalf("not implemented")
	return protoreflect.Value{}
}

// WhichOneof reports which field within the oneof is populated,
// returning nil if none are populated.
// It panics if the oneof descriptor does not belong to this message.
func (a *adapter) WhichOneof(protoreflect.OneofDescriptor) protoreflect.FieldDescriptor {
	klog.Fatalf("not implemented")
	return nil
}

// GetUnknown retrieves the entire list of unknown fields.
// The caller may only mutate the contents of the RawFields
// if the mutated bytes are stored back into the message with SetUnknown.
func (a *adapter) GetUnknown() protoreflect.RawFields {
	klog.Infof("GetUnknown not implemented")
	return nil
}

// SetUnknown stores an entire list of unknown fields.
// The raw fields must be syntactically valid according to the wire format.
// An implementation may panic if this is not the case.
// Once stored, the caller must not mutate the content of the RawFields.
// An empty RawFields may be passed to clear the fields.
//
// SetUnknown is a mutating operation and unsafe for concurrent use.
func (a *adapter) SetUnknown(protoreflect.RawFields) {
	klog.Fatalf("SetUnknown not implemented")
}

// IsValid reports whether the message is valid.
//
// An invalid message is an empty, read-only value.
//
// An invalid message often corresponds to a nil pointer of the concrete
// message type, but the details are implementation dependent.
// Validity is not part of the protobuf data model, and may not
// be preserved in marshaling or other operations.
func (a *adapter) IsValid() bool {
	klog.Infof("TODO: IsValid")
	return true
}

// ProtoMethods returns optional fast-path implementions of various operations.
// This method may return nil.
//
// The returned methods type is identical to
// "google.golang.org/protobuf/runtime/protoiface".Methods.
// Consult the protoiface package documentation for details.
func (a *adapter) ProtoMethods() *protoiface.Methods {
	return &protoiface.Methods{
		Marshal:          a.marshal,
		Unmarshal:        a.unmarshal,
		CheckInitialized: a.checkInitialized,
	}
}

func (a *adapter) marshal(input protoiface.MarshalInput) (protoiface.MarshalOutput, error) {
	if input.Flags != 0 {
		return protoiface.MarshalOutput{}, fmt.Errorf("flags not supported: %+v", input)
	}

	buffer := input.Buf
	size := a.msg.Size()
	if len(buffer) > size {
		buffer = buffer[:size]
	}
	if len(buffer) < size {
		buffer = make([]byte, size)
	}
	_, err := a.msg.MarshalToSizedBuffer(buffer)
	if err != nil {
		return protoiface.MarshalOutput{}, err
	}
	return protoiface.MarshalOutput{
		Buf: buffer,
	}, nil
}

func (a *adapter) unmarshal(input protoiface.UnmarshalInput) (protoiface.UnmarshalOutput, error) {
	if input.Flags != 0 {
		return protoiface.UnmarshalOutput{}, fmt.Errorf("flags not supported: %+v", input)
	}

	err := a.msg.Unmarshal(input.Buf)
	if err != nil {
		return protoiface.UnmarshalOutput{}, err
	}
	return protoiface.UnmarshalOutput{}, nil
}

func (a *adapter) checkInitialized(input protoiface.CheckInitializedInput) (protoiface.CheckInitializedOutput, error) {
	klog.Infof("checkInitialized should check that fields are initialized")
	return protoiface.CheckInitializedOutput{}, nil
}

// type MessageV1 interface {
// 	Reset()
// 	String() string
// 	ProtoMessage()
// }

func (a *adapter) Reset() {
	a.msg.Reset()
}

func (a *adapter) String() string {
	return prototext.MarshalOptions{Multiline: false}.Format(a)
}

func (a *adapter) ProtoMessage() {
	klog.Fatalf("ProtoMessage not implemented")
}

type listAdapter struct {
	slice reflect.Value
}

var _ protoreflect.List = &listAdapter{}

// Len reports the number of entries in the List.
// Get, Set, and Truncate panic with out of bound indexes.
func (a *listAdapter) Len() int {
	return a.slice.Len()
}

// Get retrieves the value at the given index.
// It never returns an invalid value.
func (a *listAdapter) Get(int) protoreflect.Value {
	klog.Fatalf("not implemented")
	return protoreflect.Value{}
}

// Set stores a value for the given index.
// When setting a composite type, it is unspecified whether the set
// value aliases the source's memory in any way.
//
// Set is a mutating operation and unsafe for concurrent use.
func (a *listAdapter) Set(int, protoreflect.Value) {
	klog.Fatalf("not implemented")

}

// Append appends the provided value to the end of the list.
// When appending a composite type, it is unspecified whether the appended
// value aliases the source's memory in any way.
//
// Append is a mutating operation and unsafe for concurrent use.
func (a *listAdapter) Append(protoreflect.Value) {
	klog.Fatalf("not implemented")

}

// AppendMutable appends a new, empty, mutable message value to the end
// of the list and returns it.
// It panics if the list does not contain a message type.
func (a *listAdapter) AppendMutable() protoreflect.Value {
	klog.Fatalf("not implemented")
	return protoreflect.Value{}
}

// Truncate truncates the list to a smaller length.
//
// Truncate is a mutating operation and unsafe for concurrent use.
func (a *listAdapter) Truncate(int) {
	klog.Fatalf("not implemented")

}

// NewElement returns a new value for a list element.
// For enums, this returns the first enum value.
// For other scalars, this returns the zero value.
// For messages, this returns a new, empty, mutable value.
func (a *listAdapter) NewElement() protoreflect.Value {
	klog.Fatalf("not implemented")
	return protoreflect.Value{}
}

// IsValid reports whether the list is valid.
//
// An invalid list is an empty, read-only value.
//
// Validity is not part of the protobuf data model, and may not
// be preserved in marshaling or other operations.
func (a *listAdapter) IsValid() bool {
	klog.Fatalf("not implemented")
	return false
}

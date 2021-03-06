package ldap

import (
	"errors"

	"github.com/gostores/encoding/asn1"
)

// SimpleBindRequest represents a username/password bind operation
type SimpleBindRequest struct {
	// Username is the name of the Directory object that the client wishes to bind as
	Username string
	// Password is the credentials to bind with
	Password string
	// Controls are optional controls to send with the bind request
	Controls []Control
	// AllowEmptyPassword sets whether the client allows binding with an empty password
	// (normally used for unauthenticated bind).
	AllowEmptyPassword bool
}

// SimpleBindResult contains the response from the server
type SimpleBindResult struct {
	Controls []Control
}

// NewSimpleBindRequest returns a bind request
func NewSimpleBindRequest(username string, password string, controls []Control) *SimpleBindRequest {
	return &SimpleBindRequest{
		Username:           username,
		Password:           password,
		Controls:           controls,
		AllowEmptyPassword: false,
	}
}

func (bindRequest *SimpleBindRequest) encode() *asn1.Packet {
	request := asn1.Encode(asn1.ClassApplication, asn1.TypeConstructed, ApplicationBindRequest, nil, "Bind Request")
	request.AppendChild(asn1.NewInteger(asn1.ClassUniversal, asn1.TypePrimitive, asn1.TagInteger, 3, "Version"))
	request.AppendChild(asn1.NewString(asn1.ClassUniversal, asn1.TypePrimitive, asn1.TagOctetString, bindRequest.Username, "User Name"))
	request.AppendChild(asn1.NewString(asn1.ClassContext, asn1.TypePrimitive, 0, bindRequest.Password, "Password"))

	request.AppendChild(encodeControls(bindRequest.Controls))

	return request
}

// SimpleBind performs the simple bind operation defined in the given request
func (l *Conn) SimpleBind(simpleBindRequest *SimpleBindRequest) (*SimpleBindResult, error) {
	if simpleBindRequest.Password == "" && !simpleBindRequest.AllowEmptyPassword {
		return nil, NewError(ErrorEmptyPassword, errors.New("ldap: empty password not allowed by the client"))
	}

	packet := asn1.Encode(asn1.ClassUniversal, asn1.TypeConstructed, asn1.TagSequence, nil, "LDAP Request")
	packet.AppendChild(asn1.NewInteger(asn1.ClassUniversal, asn1.TypePrimitive, asn1.TagInteger, l.nextMessageID(), "MessageID"))
	encodedBindRequest := simpleBindRequest.encode()
	packet.AppendChild(encodedBindRequest)

	if l.Debug {
		asn1.PrintPacket(packet)
	}

	msgCtx, err := l.sendMessage(packet)
	if err != nil {
		return nil, err
	}
	defer l.finishMessage(msgCtx)

	packetResponse, ok := <-msgCtx.responses
	if !ok {
		return nil, NewError(ErrorNetwork, errors.New("ldap: response channel closed"))
	}
	packet, err = packetResponse.ReadPacket()
	l.Debug.Printf("%d: got response %p", msgCtx.id, packet)
	if err != nil {
		return nil, err
	}

	if l.Debug {
		if err := addLDAPDescriptions(packet); err != nil {
			return nil, err
		}
		asn1.PrintPacket(packet)
	}

	result := &SimpleBindResult{
		Controls: make([]Control, 0),
	}

	if len(packet.Children) == 3 {
		for _, child := range packet.Children[2].Children {
			result.Controls = append(result.Controls, DecodeControl(child))
		}
	}

	resultCode, resultDescription := getLDAPResultCode(packet)
	if resultCode != 0 {
		return result, NewError(resultCode, errors.New(resultDescription))
	}

	return result, nil
}

// Bind performs a bind with the given username and password.
//
// It does not allow unauthenticated bind (i.e. empty password). Use the UnauthenticatedBind method
// for that.
func (l *Conn) Bind(username, password string) error {
	req := &SimpleBindRequest{
		Username:           username,
		Password:           password,
		AllowEmptyPassword: false,
	}
	_, err := l.SimpleBind(req)
	return err
}

// UnauthenticatedBind performs an unauthenticated bind.
//
// A username may be provided for trace (e.g. logging) purpose only, but it is normally not
// authenticated or otherwise validated by the LDAP server.
//
// See https://tools.ietf.org/html/rfc4513#section-5.1.2 .
// See https://tools.ietf.org/html/rfc4513#section-6.3.1 .
func (l *Conn) UnauthenticatedBind(username string) error {
	req := &SimpleBindRequest{
		Username:           username,
		Password:           "",
		AllowEmptyPassword: true,
	}
	_, err := l.SimpleBind(req)
	return err
}

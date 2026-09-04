package gorfc

// #include <sapnwrfc.h>
import "C"

import "strings"

// ConnectionAttributes is what the system reported about a connection, keyed
// by the RFC_ATTRIBUTES field names.
type ConnectionAttributes map[string]string

// wrapConnectionAttributes converts RFC_ATTRIBUTES to a map. Each field's
// length comes from the array itself rather than a literal, so a field that
// changes width cannot silently start truncating.
func wrapConnectionAttributes(attributes C.RFC_ATTRIBUTES, strip bool) (ConnectionAttributes, error) {
	connAttr := make(ConnectionAttributes, 24)
	var firstErr error

	put := func(key string, field *C.SAP_UC, size int) {
		// size counts the NUL terminator the SDK reserves. These are fixed
		// buffers, so the value is followed by NULs whether or not the caller
		// asked for stripping -- rstrip governs the blank padding of an ABAP
		// field, and a NUL is not that. The reference returned the NULs, and
		// nothing wants them.
		s, err := nWrapString(field, C.uint(size-1), strip)
		if err != nil && firstErr == nil {
			firstErr = err
		}
		connAttr[key] = strings.TrimRight(s, "\x00")
	}

	put("dest", &attributes.dest[0], len(attributes.dest))
	put("host", &attributes.host[0], len(attributes.host))
	put("partnerHost", &attributes.partnerHost[0], len(attributes.partnerHost))
	put("sysNumber", &attributes.sysNumber[0], len(attributes.sysNumber))
	put("sysId", &attributes.sysId[0], len(attributes.sysId))
	put("client", &attributes.client[0], len(attributes.client))
	put("user", &attributes.user[0], len(attributes.user))
	put("language", &attributes.language[0], len(attributes.language))
	put("trace", &attributes.trace[0], len(attributes.trace))
	put("isoLanguage", &attributes.isoLanguage[0], len(attributes.isoLanguage))
	put("codepage", &attributes.codepage[0], len(attributes.codepage))
	put("partnerCodepage", &attributes.partnerCodepage[0], len(attributes.partnerCodepage))
	put("rfcRole", &attributes.rfcRole[0], len(attributes.rfcRole))
	put("type", &attributes._type[0], len(attributes._type))
	put("partnerType", &attributes.partnerType[0], len(attributes.partnerType))
	put("rel", &attributes.rel[0], len(attributes.rel))
	put("partnerRel", &attributes.partnerRel[0], len(attributes.partnerRel))
	put("kernelRel", &attributes.kernelRel[0], len(attributes.kernelRel))
	put("cpicConvId", &attributes.cpicConvId[0], len(attributes.cpicConvId))
	put("progName", &attributes.progName[0], len(attributes.progName))
	put("partnerBytesPerChar", &attributes.partnerBytesPerChar[0], len(attributes.partnerBytesPerChar))
	put("partnerSystemCodepage", &attributes.partnerSystemCodepage[0], len(attributes.partnerSystemCodepage))
	put("partnerIP", &attributes.partnerIP[0], len(attributes.partnerIP))
	put("partnerIPv6", &attributes.partnerIPv6[0], len(attributes.partnerIPv6))

	if firstErr != nil {
		return nil, firstErr
	}
	return connAttr, nil
}

package internal

import (
	"encoding/xml"
)

func NewPrivilege(name xml.Name) Privilege {
	return Privilege{
		Raw: NewRawXMLElement(name, nil, nil),
	}
}

type Privilege struct {
	XMLName xml.Name     `xml:"DAV: privilege"`
	Raw     *RawXMLValue `xml:",any"`
}

func (p Privilege) Is(target xml.Name) bool {
	got, ok := p.Raw.XMLName()
	return ok && got == target
}

var (
	/*
	   rfc3744#section-3.1

	   The read privilege controls methods that return information about the
	   state of the resource, including the resource's properties.  Affected
	   methods include GET and PROPFIND.  Any implementation-defined
	   privilege that also controls access to GET and PROPFIND must be
	   aggregated under DAV:read - if an ACL grants access to DAV:read, the
	   client may expect that no other privilege needs to be granted to have
	   access to GET and PROPFIND.  Additionally, the read privilege MUST
	   control the OPTIONS method.
	*/
	Read = xml.Name{"DAV:", "read"}

	/*
	   rfc3744#section-3.2

	   The write privilege controls methods that lock a resource or modify
	   the content, dead properties, or (in the case of a collection)
	   membership of the resource, such as PUT and PROPPATCH.  Note that
	   state modification is also controlled via locking (see section 5.3 of
	   [RFC2518]), so effective write access requires that both write
	   privileges and write locking requirements are satisfied.  Any
	   implementation-defined privilege that also controls access to methods
	   modifying content, dead properties or collection membership must be
	   aggregated under DAV:write, e.g., if an ACL grants access to
	   DAV:write, the client may expect that no other privilege needs to be
	   granted to have access to PUT and PROPPATCH.
	*/
	Write = xml.Name{"DAV:", "write"}

	/*
	   rfc3744#section-3.3

	   The DAV:write-properties privilege controls methods that modify the
	   dead properties of the resource, such as PROPPATCH.  Whether this
	   privilege may be used to control access to any live properties is
	   determined by the implementation.  Any implementation-defined
	   privilege that also controls access to methods modifying dead
	   properties must be aggregated under DAV:write-properties - e.g., if
	   an ACL grants access to DAV:write-properties, the client can safely
	   expect that no other privilege needs to be granted to have access to
	   PROPPATCH.
	*/
	WriteProperties = xml.Name{"DAV:", "write-properties"}

	/*
	   rfc3744#section-3.4

	   The DAV:write-content privilege controls methods that modify the
	   content of an existing resource, such as PUT.  Any implementation-
	   defined privilege that also controls access to content must be
	   aggregated under DAV:write-content - e.g., if an ACL grants access to
	   DAV:write-content, the client can safely expect that no other
	   privilege needs to be granted to have access to PUT.  Note that PUT -
	   when applied to an unmapped URI - creates a new resource and
	   therefore is controlled by the DAV:bind privilege on the parent
	   collection.
	*/
	WriteContent = xml.Name{"DAV:", "write-content"}

	/*
	   rfc3744#section-3.5

	   The DAV:unlock privilege controls the use of the UNLOCK method by a
	   principal other than the lock owner (the principal that created a
	   lock can always perform an UNLOCK).  While the set of users who may
	   lock a resource is most commonly the same set of users who may modify
	   a resource, servers may allow various kinds of administrators to
	   unlock resources locked by others.  Any privilege controlling access
	   by non-lock owners to UNLOCK MUST be aggregated under DAV:unlock.

	   A lock owner can always remove a lock by issuing an UNLOCK with the
	   correct lock token and authentication credentials.  That is, even if
	   a principal does not have DAV:unlock privilege, they can still remove
	   locks they own.  Principals other than the lock owner can remove a
	   lock only if they have DAV:unlock privilege and they issue an UNLOCK
	   with the correct lock token.  Lock timeout is not affected by the
	   DAV:unlock privilege.
	*/
	Unlock = xml.Name{"DAV:", "unlock"}

	/*
	   rfc3744#section-3.6

	   The DAV:read-acl privilege controls the use of PROPFIND to retrieve
	   the DAV:acl property of the resource.
	*/
	ReadACL = xml.Name{"DAV:", "read-acl"}

	/*
	   rfc3744#section-3.7

	   The DAV:read-current-user-privilege-set privilege controls the use of
	   PROPFIND to retrieve the DAV:current-user-privilege-set property of
	   the resource.

	   Clients are intended to use this property to visually indicate in
	   their UI items that are dependent on the permissions of a resource,
	   for example, by graying out resources that are not writable.

	   This privilege is separate from DAV:read-acl because there is a need
	   to allow most users access to the privileges permitted the current
	   user (due to its use in creating the UI), while the full ACL contains
	   information that may not be appropriate for the current authenticated
	   user.  As a result, the set of users who can view the full ACL is
	   expected to be much smaller than those who can read the current user
	   privilege set, and hence distinct privileges are needed for each.
	*/
	ReadCurrentUserPrivilegeSet = xml.Name{"DAV:", "read-current-user-privilege-set"}

	/*
	   rfc3744#section-3.8

	   The DAV:write-acl privilege controls use of the ACL method to modify
	   the DAV:acl property of the resource.
	*/
	WriteACL = xml.Name{"DAV:", "write-acl"}

	/*
	   rfc3744#section-3.9

	   The DAV:bind privilege allows a method to add a new member URL to the
	   specified collection (for example via PUT or MKCOL).  It is ignored
	   for resources that are not collections.
	*/
	Bind = xml.Name{"DAV:", "bind"}

	/*
	   rfc3744#section-3.10

	   The DAV:unbind privilege allows a method to remove a member URL from
	   the specified collection (for example via DELETE or MOVE).  It is
	   ignored for resources that are not collections.
	*/
	Unbind = xml.Name{"DAV:", "unbind"}

	/*
	   rfc3744#section-3.11

	   DAV:all is an aggregate privilege that contains the entire set of
	   privileges that can be applied to the resource.
	*/
	All = xml.Name{"DAV:", "all"}
)

/*
rfc3744#section-4

rfc3744#section-5.5.1

The current user matches DAV:href only if that user is authenticated
as being (or being a member of) the principal identified by the URL
contained by that DAV:href.

The current user always matches DAV:all.

The current user matches DAV:authenticated only if authenticated.

The current user matches DAV:unauthenticated only if not
authenticated.
*/
type Principal struct {
	XMLName xml.Name     `xml:"DAV: principal"`
	Raw     *RawXMLValue `xml:",any"`
}

/*
rfc3744#section-4.1

This protected property, if non-empty, contains the URIs of network
resources with additional descriptive information about the
principal.  This property identifies additional network resources
(i.e., it contains one or more URIs) that may be consulted by a
client to gain additional knowledge concerning a principal.  One
expected use for this property is the storage of an LDAP [RFC2255]
scheme URL.  A user-agent encountering an LDAP URL could use LDAP
[RFC2251] to retrieve additional machine-readable directory
information about the principal, and display that information in its
user interface.  Support for this property is REQUIRED, and the value
is empty if no alternate URI exists for the principal.
*/
type AlternateURISet struct {
	XMLName xml.Name `xml:"DAV: alternate-URI-set"`
	Href    []Href   `xml:"href,omitempty"`
}

/*
rfc3744#section-4.2

A principal may have many URLs, but there must be one "principal URL"
that clients can use to uniquely identify a principal.  This
protected property contains the URL that MUST be used to identify
this principal in an ACL request.  Support for this property is
REQUIRED.
*/
type PrincipalURL struct {
	XMLName xml.Name `xml:"DAV: principal-URL"`
	Href    Href     `xml:"href,omitempty"`
}

/*
rfc3744#section-4.3

This property of a group principal identifies the principals that are
direct members of this group.  Since a group may be a member of
another group, a group may also have indirect members (i.e., the
members of its direct members).  A URL in the DAV:group-member-set
for a principal MUST be the DAV:principal-URL of that principal.
*/
type GroupMemberSet struct {
	XMLName xml.Name `xml:"DAV: group-member-set"`
	Href    []Href   `xml:"href,omitempty"`
}

/*
rfc3744#section-4.4

This protected property identifies the groups in which the principal
is directly a member.  Note that a server may allow a group to be a
member of another group, in which case the DAV:group-membership of
those other groups would need to be queried in order to determine the
groups in which the principal is indirectly a member.  Support for
this property is REQUIRED.
*/
type GroupMembership struct {
	XMLName xml.Name `xml:"DAV: group-membership"`
	Href    []Href   `xml:"href,omitempty"`
}

/*
rfc3744#section-5.1

This  property identifies a particular principal as being the "owner"
of the resource.  Since the owner of a resource often has special
access control capabilities (e.g., the owner frequently has permanent
DAV:write-acl privilege), clients might display the resource owner in
their user interface.

Servers MAY implement DAV:owner as protected property and MAY return
an empty DAV:owner element as property value in case no owner
information is available.
*/
type Owner struct {
	XMLName xml.Name `xml:"DAV: owner"`
	Href    Href     `xml:"href,omitempty"`
}

/*
rfc3744#section-5.2

This property identifies a particular principal as being the "group"
of the resource.  This property is commonly found on repositories
that implement the Unix privileges model.

Servers MAY implement DAV:group as protected property and MAY return
an empty DAV:group element as property value in case no group
information is available.
*/
type Group struct {
	XMLName xml.Name `xml:"DAV: group"`
	Href    Href     `xml:"href,omitempty"`
}

/*
rfc3744#section-5.3

This is a protected property that identifies the privileges defined
for the resource.
*/
type SupportedPrivilegeSet struct {
	XMLName            xml.Name             `xml:"DAV: supported-privilege-set"`
	SupportedPrivilege []SupportedPrivilege `xml:"supported-privilege"`
}

/*
rfc3744#section-5.3

Each privilege appears as an XML element, where aggregate privileges
list as sub-elements all of the privileges that they aggregate.
*/
type SupportedPrivilege struct {
	XMLName   xml.Name  `xml:"DAV: supported-privilege"`
	Privilege Privilege `xml:"privilege"`
	/*
		Abstract will be nil if not set

		rfc3744#section-5.3

		An abstract privilege MUST NOT be used in an ACE for that resource.
		Servers MUST fail an attempt to set an abstract privilege.
	*/
	Abstract           *struct{}            `xml:"abstract,omitempty"`
	Description        Description          `xml:"description"`
	SupportedPrivilege []SupportedPrivilege `xml:"supported-privilege"`
}

/*
rfc3744#section-5.3

A description is a human-readable description of what this privilege
controls access to.  Servers MUST indicate the human language of the
description using the xml:lang attribute and SHOULD consider the HTTP
Accept-Language request header when selecting one of multiple
available languages.
*/
type Description struct {
	XMLName xml.Name `xml:"DAV: description"`
	Text    string   `xml:",chardata"`
	Lang    string   `xml:"lang,attr,omitempty"`
}

/*
rfc3744#section-5.4

DAV:current-user-privilege-set is a protected property containing the
exact set of privileges (as computed by the server) granted to the
currently authenticated HTTP user.  Aggregate privileges and their
contained privileges are listed.  A user-agent can use the value of
this property to adjust its user interface to make actions
inaccessible (e.g., by graying out a menu item or button) for which
the current principal does not have permission.  This property is
also useful for determining what operations the current principal can
perform, without having to actually execute an operation.
*/
type CurrentUserPrivilegeSet struct {
	XMLName   xml.Name    `xml:"DAV: current-user-privilege-set"`
	Privilege []Privilege `xml:"privilege"`
}

// convenience CurrentUserPrivilegeSet
var (
	CurrentUserPrivilegeSetReadOnly = CurrentUserPrivilegeSet{
		Privilege: []Privilege{NewPrivilege(Read)},
	}
	CurrentUserPrivilegeSetReadWrite = CurrentUserPrivilegeSet{
		Privilege: []Privilege{NewPrivilege(Read), NewPrivilege(Write)},
	}
)

/*
rfc3744#section-5.5

This is a protected property that specifies the list of access
control entries (ACEs), which define what principals are to get what
privileges for this resource.
*/
type ACL struct {
	XMLName xml.Name `xml:"DAV: acl"`
	ACE     []ACE    `xml:"ace"`
}

/*
rfc3744#section-5.5

Each DAV:ace element specifies the set of privileges to be either
granted or denied to a single principal.  If the DAV:acl property is
empty, no principal is granted any privilege.
*/
type ACE struct {
	XMLName xml.Name `xml:"DAV: ace"`
	/*
	   rfc3744#section-5.5.1

	   The DAV:principal element identifies the principal to which this ACE
	   applies.
	*/
	Principal Principal `xml:"principal,omitempty"`
	Grant     *Grant    `xml:"grant,omitempty"`
	Deny      *Deny     `xml:"deny,omitempty"`
}

/*
rfc3744#section-5.5.2

Each DAV:grant or DAV:deny element specifies the set of privileges to
be either granted or denied to the specified principal.  A DAV:grant
or DAV:deny element of the DAV:acl of a resource MUST only contain
non-abstract elements specified in the DAV:supported-privilege-set of
that resource.
*/
type Grant struct {
	XMLName   xml.Name    `xml:"DAV: grant"`
	Privilege []Privilege `xml:"privilege"`
}

/*
rfc3744#section-5.5.2

Each DAV:grant or DAV:deny element specifies the set of privileges to
be either granted or denied to the specified principal.  A DAV:grant
or DAV:deny element of the DAV:acl of a resource MUST only contain
non-abstract elements specified in the DAV:supported-privilege-set of
that resource.
*/
type Deny struct {
	XMLName   xml.Name    `xml:"DAV: deny"`
	Privilege []Privilege `xml:"privilege"`
}

// to be continued (5.5.2. & following)
///////////////////////////////////////////////

var (
	/*
	   rfc3744#section-5.6.1

	   This element indicates that ACEs with deny clauses are not allowed.
	*/
	GrantOnly = xml.Name{"DAV:", "grant-only"}
	/*
	   rfc3744#section-5.6.2

	   This element indicates that ACEs with the <invert> element are not
	   allowed.
	*/
	NoInvert = xml.Name{"DAV:", "no-invert"}
	/*
	   rfc3744#section-5.6.3

	   This element indicates that all deny ACEs must precede all grant
	   ACEs.
	*/
	DenyBeforeGrant = xml.Name{"DAV:", "deny-before-grant"}
)

var (
	/*
	   rfc3744#section-8.1.1

	   The ACEs submitted in the ACL request MUST NOT
	   conflict with each other.  This is a catchall error code indicating
	   that an implementation-specific ACL restriction has been violated.
	*/
	NoACEConflict = xml.Name{"DAV:", "no-ace-conflict"}

	/*
	   rfc3744#section-8.1.1

	   The ACEs submitted in the ACL
	   request MUST NOT conflict with the protected ACEs on the resource.
	   For example, if the resource has a protected ACE granting DAV:write
	   to a given principal, then it would not be consistent if the ACL
	   request submitted an ACE denying DAV:write to the same principal.
	*/
	NoProtectedACEConflict = xml.Name{"DAV:", "no-protected-ace-conflict"}

	/*
	   rfc3744#section-8.1.1

	   The ACEs submitted in the ACL
	   request MUST NOT conflict with the inherited ACEs on the resource.
	   For example, if the resource inherits an ACE from its parent
	   collection granting DAV:write to a given principal, then it would not
	   be consistent if the ACL request submitted an ACE denying DAV:write
	   to the same principal.  Note that reporting of this error will be
	   implementation-dependent.  Implementations MUST either report this
	   error or allow the ACE to be set, and then let normal ACE evaluation
	   rules determine whether the new ACE has any impact on the privileges
	   available to a specific principal.
	*/
	NoInheritedACEConflict = xml.Name{"DAV:", "no-inherited-ace-conflict"}

	/*
	   rfc3744#section-8.1.1

	   The number of ACEs submitted in the ACL
	   request MUST NOT exceed the number of ACEs allowed on that resource.
	   However, ACL-compliant servers MUST support at least one ACE granting
	   privileges to a single principal, and one ACE granting privileges to
	   a group.
	*/
	LimitedNumberOfACEs = xml.Name{"DAV:", "limited-number-of-aces"}

	// already defined above:
	// DenyBeforeGrant
	// GrantOnly
	// NoInvert

	/*
	   rfc3744#section-8.1.1

	   The ACL request MUST NOT attempt to grant or deny
	   an abstract privilege
	*/
	NoAbstract = xml.Name{"DAV:", "no-abstract"}

	/*
	   rfc3744#section-8.1.1

	   The ACEs submitted in the ACL request
	   MUST be supported by the resource.
	*/
	NotSupportedPrivilege = xml.Name{"DAV:", "not-supported-privilege"}

	/*
	   rfc3744#section-8.1.1

	   The result of the ACL request MUST
	   have at least one ACE for each principal identified in a
	   DAV:required-principal XML element in the ACL semantics of that
	   resource
	*/
	MissingRequiredPrincipal = xml.Name{"DAV:", "missing-required-principal"}

	/*
	   rfc3744#section-8.1.1

	   Every principal URL in the ACL request
	   MUST identify a principal resource.
	*/
	RecognizedPrincipal = xml.Name{"DAV:", "recognized-principal"}

	/*
	   rfc3744#section-8.1.1

	   The principals specified in the ACEs
	   submitted in the ACL request MUST be allowed as principals for the
	   resource.  For example, a server where only authenticated principals
	   can access resources would not allow the DAV:all or
	   DAV:unauthenticated principals to be used in an ACE, since these
	   would allow unauthenticated access to resources.
	*/
	AllowedPrincipal = xml.Name{"DAV:", "allowed-principal"}
)

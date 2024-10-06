package internal

import (
	"encoding/xml"
	"fmt"
	"strings"
	"testing"
)

func decodePropInsideMultiStatus(data []byte, v interface{}) error {
	var ms MultiStatus
	err := xml.Unmarshal(data, &ms)
	if err != nil {
		return err
	}
	if len(ms.Responses) != 1 {
		return fmt.Errorf("expected 1 <response>, got %d", len(ms.Responses))
	}
	ps := ms.Responses[0].PropStats
	if len(ps) != 1 {
		return fmt.Errorf("expected 1 <propstat>, got %d", len(ps))
	}
	return ps[0].Prop.Decode(v)
}

func checkSupportedPrivilege(t *testing.T, sp SupportedPrivilege, privilege xml.Name, abstract bool, description string, children int) {
	t.Helper()

	if !sp.Privilege.Is(privilege) {
		t.Errorf("expected %s, got %v", privilege, sp.Privilege.Raw)
	}
	if abstract {
		if sp.Abstract == nil {
			t.Errorf("missing expected <abstract>")
		}
	} else {
		if sp.Abstract != nil {
			t.Errorf("unexpected <abstract>")
		}
	}
	if strings.TrimSpace(sp.Description.Text) != description {
		t.Errorf("expected description %q, got %q", description, strings.TrimSpace(sp.Description.Text))
	}
	if strings.TrimSpace(sp.Description.Lang) != "en" {
		t.Errorf("expected lang %q, got %q", "en", sp.Description.Lang)
	}
	if len(sp.SupportedPrivileges) != children {
		t.Fatalf("expected %d <supported-privilege>, got %d", children, len(sp.SupportedPrivileges))
	}
}

func TestACLMarshalling(t *testing.T) {
	/* rfc3744#section-5.1.1 */
	t.Run("owner", func(t *testing.T) {
		var owner Owner
		err := decodePropInsideMultiStatus([]byte(` <?xml version="1.0" encoding="utf-8" ?>
   <D:multistatus xmlns:D="DAV:">
     <D:response>
       <D:href>http://www.example.com/papers/</D:href>
       <D:propstat>
         <D:prop>
           <D:owner>
             <D:href>http://www.example.com/acl/users/gstein</D:href>
           </D:owner>
         </D:prop>
         <D:status>HTTP/1.1 200 OK</D:status>
       </D:propstat>
     </D:response>
   </D:multistatus>`), &owner)
		if err != nil {
			t.Error(err)
		}
		if owner.Href.String() != "http://www.example.com/acl/users/gstein" {
			t.Fatalf("expected http://www.example.com/acl/users/gstein, got %s", owner.Href.String())
		}
	})

	/* rfc3744#section-5.3.1 */
	t.Run("supported-privilege-set", func(t *testing.T) {
		var sps SupportedPrivilegeSet
		err := decodePropInsideMultiStatus([]byte(` <?xml version="1.0" encoding="utf-8" ?>
   <D:multistatus xmlns:D="DAV:">
     <D:response>
       <D:href>http://www.example.com/papers/</D:href>
       <D:propstat>
         <D:prop>
           <D:supported-privilege-set>
             <D:supported-privilege>
               <D:privilege><D:all/></D:privilege>
              <D:abstract/>
               <D:description xml:lang="en">
                 Any operation
               </D:description>
               <D:supported-privilege>
                 <D:privilege><D:read/></D:privilege>
                 <D:description xml:lang="en">
                   Read any object
                 </D:description>
                 <D:supported-privilege>
                   <D:privilege><D:read-acl/></D:privilege>
                   <D:abstract/>
                   <D:description xml:lang="en">Read ACL</D:description>
                 </D:supported-privilege>
                 <D:supported-privilege>
                   <D:privilege>
                     <D:read-current-user-privilege-set/>
                   </D:privilege>
                   <D:abstract/>
                   <D:description xml:lang="en">
                     Read current user privilege set property
                   </D:description>
                 </D:supported-privilege>
               </D:supported-privilege>
               <D:supported-privilege>
                 <D:privilege><D:write/></D:privilege>
                 <D:description xml:lang="en">
                   Write any object
                 </D:description>
                 <D:supported-privilege>
                   <D:privilege><D:write-acl/></D:privilege>
                   <D:description xml:lang="en">
                     Write ACL
                   </D:description>
                   <D:abstract/>
                 </D:supported-privilege>
                 <D:supported-privilege>
                   <D:privilege><D:write-properties/></D:privilege>
                   <D:description xml:lang="en">
                     Write properties
                   </D:description>
                 </D:supported-privilege>
                 <D:supported-privilege>
                   <D:privilege><D:write-content/></D:privilege>
                   <D:description xml:lang="en">
                     Write resource content
                   </D:description>
                 </D:supported-privilege>
               </D:supported-privilege>
               <D:supported-privilege>
                 <D:privilege><D:unlock/></D:privilege>
                 <D:description xml:lang="en">
                   Unlock resource
                 </D:description>
               </D:supported-privilege>
             </D:supported-privilege>
           </D:supported-privilege-set>
         </D:prop>
         <D:status>HTTP/1.1 200 OK</D:status>
       </D:propstat>
     </D:response>
   </D:multistatus>`), &sps)
		if err != nil {
			t.Error(err)
		}
		if len(sps.SupportedPrivileges) != 1 {
			t.Fatalf("expected 1 <supported-privilege>, got %d", len(sps.SupportedPrivileges))
		}
		sp := sps.SupportedPrivileges[0]

		checkSupportedPrivilege(t, sp, All, true, "Any operation", 3)

		checkSupportedPrivilege(t, sp.SupportedPrivileges[0], Read, false, "Read any object", 2)
		checkSupportedPrivilege(t, sp.SupportedPrivileges[1], Write, false, "Write any object", 3)
		checkSupportedPrivilege(t, sp.SupportedPrivileges[2], Unlock, false, "Unlock resource", 0)

		checkSupportedPrivilege(t, sp.SupportedPrivileges[0].SupportedPrivileges[0], ReadAcl, true, "Read ACL", 0)
		checkSupportedPrivilege(t, sp.SupportedPrivileges[0].SupportedPrivileges[1], ReadCurrentUserPrivilegeSet, true, "Read current user privilege set property", 0)

		checkSupportedPrivilege(t, sp.SupportedPrivileges[1].SupportedPrivileges[0], WriteAcl, true, "Write ACL", 0)
		checkSupportedPrivilege(t, sp.SupportedPrivileges[1].SupportedPrivileges[1], WriteProperties, false, "Write properties", 0)
		checkSupportedPrivilege(t, sp.SupportedPrivileges[1].SupportedPrivileges[2], WriteContent, false, "Write resource content", 0)

		sp = SupportedPrivilege{
			Privilege:   NewPrivilege(All),
			Abstract:    &struct{}{},
			Description: Description{Text: "all"},
		}
		buf, err := xml.Marshal(sp)
		if err != nil {
			t.Error(err)
		}
		if want := "<supported-privilege xmlns=\"DAV:\"><privilege xmlns=\"DAV:\"><all xmlns=\"DAV:\"></all></privilege><abstract></abstract><description xmlns=\"DAV:\">all</description></supported-privilege>"; string(buf) != want {
			t.Errorf("expected  %q, got %q", want, buf)
		}

		sp = SupportedPrivilege{
			Privilege:   NewPrivilege(Read),
			Abstract:    nil,
			Description: Description{Text: "read"},
		}
		buf, err = xml.Marshal(sp)
		if err != nil {
			t.Error(err)
		}
		if want := "<supported-privilege xmlns=\"DAV:\"><privilege xmlns=\"DAV:\"><read xmlns=\"DAV:\"></read></privilege><description xmlns=\"DAV:\">read</description></supported-privilege>"; string(buf) != want {
			t.Errorf("expected  %q, got %q", want, buf)
		}
	})

	/* rfc3744#section-5.4.1 */
	t.Run("current-user-privilege-set", func(t *testing.T) {
		var cups CurrentUserPrivilegeSet
		err := decodePropInsideMultiStatus([]byte(`<?xml version="1.0" encoding="utf-8" ?>
<D:multistatus xmlns:D="DAV:">
	<D:response>
	<D:href>http://www.example.com/papers/</D:href>
	<D:propstat>
	<D:prop>
		<D:current-user-privilege-set>
		<D:privilege><D:read/></D:privilege>
		</D:current-user-privilege-set>
	</D:prop>
	<D:status>HTTP/1.1 200 OK</D:status>
	</D:propstat>
	</D:response>
</D:multistatus>`), &cups)
		if err != nil {
			t.Error(err)
		}
		if len(cups.Privileges) != 1 {
			t.Fatalf("expected 1 <privilege>, got %d", len(cups.Privileges))
		}
		if !cups.Privileges[0].Is(Read) {
			t.Fatalf("expected <read>, got %v", cups.Privileges[0].Raw)
		}
	})
	/* rfc3744#section-5.5.5 */
	t.Run("acl", func(t *testing.T) {
		var acl ACL
		err := decodePropInsideMultiStatus([]byte(`<D:multistatus xmlns:D="DAV:">
     <D:response>
       <D:href>http://www.example.com/papers/</D:href>
       <D:propstat>
         <D:prop>
           <D:acl>
           <D:ace>
             <D:principal>
               <D:href
               >http://www.example.com/acl/groups/maintainers</D:href>
             </D:principal>
             <D:grant>
               <D:privilege><D:write/></D:privilege>
             </D:grant>
           </D:ace>
           <D:ace>
             <D:principal>
               <D:all/>
             </D:principal>
             <D:grant>
               <D:privilege><D:read/></D:privilege>
             </D:grant>
           </D:ace>
         </D:acl>
         </D:prop>
         <D:status>HTTP/1.1 200 OK</D:status>
       </D:propstat>
     </D:response>
   </D:multistatus>`), &acl)
		if err != nil {
			t.Error(err)
		}
		if len(acl.ACE) != 2 {
			t.Fatalf("expected 2 <ace>, got %d", len(acl.ACE))
		}
		{
			ace := acl.ACE[0]

			principalName, ok := ace.Principal.Raw.XMLName()
			if want := (xml.Name{"DAV:", "href"}); !ok || principalName != want {
				t.Fatalf("expected %s, got %s", want, principalName)
			}

			var href Href
			err = ace.Principal.Raw.Decode(&href)
			if err != nil {
				t.Error(err)
			}
			if want := "http://www.example.com/acl/groups/maintainers"; href.String() != want {
				t.Fatalf("expected %s, got %s", want, href.String())
			}

			if len(ace.Grant.Privileges) != 1 {
				t.Fatalf("expected 1 <privilege>, got %d", len(ace.Grant.Privileges))
			}

			if !ace.Grant.Privileges[0].Is(Write) {
				t.Fatalf("expected <write>, got %v", ace.Grant.Privileges[0].Raw)
			}
		}
		{
			ace := acl.ACE[1]

			principalName, ok := ace.Principal.Raw.XMLName()
			if want := (xml.Name{"DAV:", "all"}); !ok || principalName != want {
				t.Fatalf("expected %s, got %s", want, principalName)
			}

			if len(ace.Grant.Privileges) != 1 {
				t.Fatalf("expected 1 <privilege>, got %d", len(ace.Grant.Privileges))
			}

			if !ace.Grant.Privileges[0].Is(Read) {
				t.Fatalf("expected <read>, got %v", ace.Grant.Privileges[0].Raw)
			}
		}

		var ace ACE
		ace.Principal.Raw = NewRawXMLElement(xml.Name{"DAV:", "authenticated"}, nil, nil)
		ace.Grant = &Grant{
			Privileges: []Privilege{
				NewPrivilege(Read),
			},
		}
		buf, err := xml.Marshal(ace)
		if err != nil {
			t.Error(err)
		}
		if want := "<ace xmlns=\"DAV:\"><principal xmlns=\"DAV:\"><authenticated xmlns=\"DAV:\"></authenticated></principal><grant xmlns=\"DAV:\"><privilege xmlns=\"DAV:\"><read xmlns=\"DAV:\"></read></privilege></grant></ace>"; string(buf) != want {
			t.Errorf("expected  %q, got %q", want, buf)
		}
	})
}

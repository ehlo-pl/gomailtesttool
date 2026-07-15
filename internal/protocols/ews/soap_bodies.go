package ews

const getFolderSOAPBody = `    <m:GetFolder>
      <m:FolderShape>
        <t:BaseShape>Default</t:BaseShape>
      </m:FolderShape>
      <m:FolderIds>
        <t:DistinguishedFolderId Id="inbox"/>
      </m:FolderIds>
    </m:GetFolder>`

// findFolderSOAPBody lists all top-level mail folders under MsgFolderRoot.
const findFolderSOAPBody = `    <m:FindFolder Traversal="Shallow">
      <m:FolderShape>
        <t:BaseShape>Default</t:BaseShape>
      </m:FolderShape>
      <m:ParentFolderIds>
        <t:DistinguishedFolderId Id="msgfolderroot"/>
      </m:ParentFolderIds>
    </m:FindFolder>`

// findItemInboxSOAPBodyFmt is a format string for FindItem on the Inbox.
// Arg 1: MaxEntriesReturned (int).
const findItemInboxSOAPBodyFmt = `    <m:FindItem Traversal="Shallow">
      <m:ItemShape>
        <t:BaseShape>Default</t:BaseShape>
        <t:AdditionalProperties>
          <t:FieldURI FieldURI="message:From"/>
          <t:FieldURI FieldURI="item:DateTimeReceived"/>
        </t:AdditionalProperties>
      </m:ItemShape>
      <m:IndexedPageItemView MaxEntriesReturned="%d" Offset="0" BasePoint="Beginning"/>
      <m:SortOrder>
        <t:FieldOrder Order="Descending">
          <t:FieldURI FieldURI="item:DateTimeReceived"/>
        </t:FieldOrder>
      </m:SortOrder>
      <m:ParentFolderIds>
        <t:DistinguishedFolderId Id="inbox"/>
      </m:ParentFolderIds>
    </m:FindItem>`

// findItemBySubjectSOAPBodyFmt searches all items by subject substring.
// Arg 1: MaxEntriesReturned (int), Arg 2: XML-escaped subject string.
const findItemBySubjectSOAPBodyFmt = `    <m:FindItem Traversal="Shallow">
      <m:ItemShape>
        <t:BaseShape>Default</t:BaseShape>
      </m:ItemShape>
      <m:IndexedPageItemView MaxEntriesReturned="%d" Offset="0" BasePoint="Beginning"/>
      <m:Restriction>
        <t:Contains ContainmentMode="Substring" ContainmentComparison="IgnoreCase">
          <t:FieldURI FieldURI="item:Subject"/>
          <t:Constant Value="%s"/>
        </t:Contains>
      </m:Restriction>
      <m:ParentFolderIds>
        <t:DistinguishedFolderId Id="allitems"/>
      </m:ParentFolderIds>
    </m:FindItem>`

// findItemByMsgIDSOAPBodyFmt searches items by Internet Message-ID (MAPI property PR_INTERNET_MESSAGE_ID, 0x1035).
// Arg 1: MaxEntriesReturned (int), Arg 2: XML-escaped Message-ID value.
const findItemByMsgIDSOAPBodyFmt = `    <m:FindItem Traversal="Shallow">
      <m:ItemShape>
        <t:BaseShape>Default</t:BaseShape>
      </m:ItemShape>
      <m:IndexedPageItemView MaxEntriesReturned="%d" Offset="0" BasePoint="Beginning"/>
      <m:Restriction>
        <t:IsEqualTo>
          <t:ExtendedFieldURI PropertyTag="0x1035" PropertyType="String"/>
          <t:FieldURIOrConstant><t:Constant Value="%s"/></t:FieldURIOrConstant>
        </t:IsEqualTo>
      </m:Restriction>
      <m:ParentFolderIds>
        <t:DistinguishedFolderId Id="allitems"/>
      </m:ParentFolderIds>
    </m:FindItem>`

// findItemByBothSOAPBodyFmt searches by both Message-ID (exact) and subject (substring, AND).
// Args: MaxEntriesReturned, XML-escaped Message-ID, XML-escaped subject.
const findItemByBothSOAPBodyFmt = `    <m:FindItem Traversal="Shallow">
      <m:ItemShape>
        <t:BaseShape>Default</t:BaseShape>
      </m:ItemShape>
      <m:IndexedPageItemView MaxEntriesReturned="%d" Offset="0" BasePoint="Beginning"/>
      <m:Restriction>
        <t:And>
          <t:IsEqualTo>
            <t:ExtendedFieldURI PropertyTag="0x1035" PropertyType="String"/>
            <t:FieldURIOrConstant><t:Constant Value="%s"/></t:FieldURIOrConstant>
          </t:IsEqualTo>
          <t:Contains ContainmentMode="Substring" ContainmentComparison="IgnoreCase">
            <t:FieldURI FieldURI="item:Subject"/>
            <t:Constant Value="%s"/>
          </t:Contains>
        </t:And>
      </m:Restriction>
      <m:ParentFolderIds>
        <t:DistinguishedFolderId Id="allitems"/>
      </m:ParentFolderIds>
    </m:FindItem>`

// getItemMIMESOAPBodyFmt fetches raw MIME content for a single item by ID.
// Arg 1: XML-escaped ItemId.
const getItemMIMESOAPBodyFmt = `    <m:GetItem>
      <m:ItemShape>
        <t:BaseShape>IdOnly</t:BaseShape>
        <t:IncludeMimeContent>true</t:IncludeMimeContent>
        <t:AdditionalProperties>
          <t:FieldURI FieldURI="item:Subject"/>
        </t:AdditionalProperties>
      </m:ItemShape>
      <m:ItemIds>
        <t:ItemId Id="%s"/>
      </m:ItemIds>
    </m:GetItem>`

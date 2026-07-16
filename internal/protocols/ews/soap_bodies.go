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

// findItemCalendarSOAPBodyFmt lists calendar events in a time window using
// CalendarView (which expands recurring meetings).
// Args: MaxEntriesReturned (int), StartDate (xs:dateTime), EndDate (xs:dateTime).
const findItemCalendarSOAPBodyFmt = `    <m:FindItem Traversal="Shallow">
      <m:ItemShape>
        <t:BaseShape>Default</t:BaseShape>
        <t:AdditionalProperties>
          <t:FieldURI FieldURI="calendar:Start"/>
          <t:FieldURI FieldURI="calendar:End"/>
          <t:FieldURI FieldURI="calendar:Organizer"/>
        </t:AdditionalProperties>
      </m:ItemShape>
      <m:CalendarView MaxEntriesReturned="%d" StartDate="%s" EndDate="%s"/>
      <m:ParentFolderIds>
        <t:DistinguishedFolderId Id="calendar"/>
      </m:ParentFolderIds>
    </m:FindItem>`

// createCalendarItemSOAPBodyFmt creates a meeting and sends invitations to all
// attendees.
// Args: XML-escaped subject, body type ("Text"/"HTML"), XML-escaped body,
// Start (xs:dateTime), End (xs:dateTime), attendee XML blocks.
const createCalendarItemSOAPBodyFmt = `    <m:CreateItem SendMeetingInvitations="SendToAllAndSaveCopy">
      <m:SavedItemFolderId>
        <t:DistinguishedFolderId Id="calendar"/>
      </m:SavedItemFolderId>
      <m:Items>
        <t:CalendarItem>
          <t:Subject>%s</t:Subject>
          <t:Body BodyType="%s">%s</t:Body>
          <t:Start>%s</t:Start>
          <t:End>%s</t:End>
          <t:RequiredAttendees>
%s      </t:RequiredAttendees>
        </t:CalendarItem>
      </m:Items>
    </m:CreateItem>`

// getUserAvailabilitySOAPBodyFmt requests merged free/busy data for a single
// attendee over a time window (UTC).
// Args: XML-escaped attendee email, window start (xs:dateTime), window end (xs:dateTime).
const getUserAvailabilitySOAPBodyFmt = `    <m:GetUserAvailabilityRequest>
      <t:TimeZone>
        <t:Bias>0</t:Bias>
        <t:StandardTime>
          <t:Bias>0</t:Bias>
          <t:Time>00:00:00</t:Time>
          <t:DayOrder>1</t:DayOrder>
          <t:Month>1</t:Month>
          <t:DayOfWeek>Sunday</t:DayOfWeek>
        </t:StandardTime>
        <t:DaylightTime>
          <t:Bias>0</t:Bias>
          <t:Time>00:00:00</t:Time>
          <t:DayOrder>1</t:DayOrder>
          <t:Month>7</t:Month>
          <t:DayOfWeek>Sunday</t:DayOfWeek>
        </t:DaylightTime>
      </t:TimeZone>
      <m:MailboxDataArray>
        <t:MailboxData>
          <t:Email>
            <t:Address>%s</t:Address>
          </t:Email>
          <t:AttendeeType>Required</t:AttendeeType>
          <t:ExcludeConflicts>false</t:ExcludeConflicts>
        </t:MailboxData>
      </m:MailboxDataArray>
      <t:FreeBusyViewOptions>
        <t:TimeWindow>
          <t:StartTime>%s</t:StartTime>
          <t:EndTime>%s</t:EndTime>
        </t:TimeWindow>
        <t:MergedFreeBusyIntervalInMinutes>60</t:MergedFreeBusyIntervalInMinutes>
        <t:RequestedView>MergedOnly</t:RequestedView>
      </t:FreeBusyViewOptions>
    </m:GetUserAvailabilityRequest>`

// getUserAvailabilityDetailedSOAPBodyFmt requests the detailed FreeBusy view
// (per-event CalendarEventArray) for a single attendee over a time window (UTC).
// Args: XML-escaped attendee email, window start (xs:dateTime), window end (xs:dateTime).
const getUserAvailabilityDetailedSOAPBodyFmt = `    <m:GetUserAvailabilityRequest>
      <t:TimeZone>
        <t:Bias>0</t:Bias>
        <t:StandardTime>
          <t:Bias>0</t:Bias>
          <t:Time>00:00:00</t:Time>
          <t:DayOrder>1</t:DayOrder>
          <t:Month>1</t:Month>
          <t:DayOfWeek>Sunday</t:DayOfWeek>
        </t:StandardTime>
        <t:DaylightTime>
          <t:Bias>0</t:Bias>
          <t:Time>00:00:00</t:Time>
          <t:DayOrder>1</t:DayOrder>
          <t:Month>7</t:Month>
          <t:DayOfWeek>Sunday</t:DayOfWeek>
        </t:DaylightTime>
      </t:TimeZone>
      <m:MailboxDataArray>
        <t:MailboxData>
          <t:Email>
            <t:Address>%s</t:Address>
          </t:Email>
          <t:AttendeeType>Required</t:AttendeeType>
          <t:ExcludeConflicts>false</t:ExcludeConflicts>
        </t:MailboxData>
      </m:MailboxDataArray>
      <t:FreeBusyViewOptions>
        <t:TimeWindow>
          <t:StartTime>%s</t:StartTime>
          <t:EndTime>%s</t:EndTime>
        </t:TimeWindow>
        <t:MergedFreeBusyIntervalInMinutes>30</t:MergedFreeBusyIntervalInMinutes>
        <t:RequestedView>FreeBusy</t:RequestedView>
      </t:FreeBusyViewOptions>
    </m:GetUserAvailabilityRequest>`

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

import{j as x}from"./jsx-runtime-BjG_zV1W.js";import{C as G,a as b}from"./ConnectorCard-BsGxnjR4.js";import"./client-Bd8K-UT9.js";import{C as s}from"./index-C7dhk8kh.js";import"./index-yIsmwZOr.js";import"./theme-D4YlWSlu.js";import"./createSimplePaletteValueFilter-DRANIdZ4.js";import"./Typography-zR6YQtWt.js";import"./Box-qKFn2-pG.js";import"./Button-Ct-3TB0V.js";const E=(e,y)=>{const S=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${y}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(S)}`},L={title:"Components/ConnectorCard",component:G,parameters:{layout:"centered"},tags:["autodocs"]},i={id:"google-calendar",version:1,state:s.ACTIVE,type:"oauth",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",highlight:"Manage events and appointments from Google Calendar.",logo:E("Google Calendar","#1a73e8"),versions:1,states:[s.ACTIVE]},n={args:{connector:i,onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},o={args:{connector:{...i,highlight:`**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:

• Event creation and management
• Meeting scheduling
• Reminder notifications
• Calendar sharing`},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},t={args:{connector:i,onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!0}},a={args:{connector:{...i,description:"This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.",highlight:"Short marketplace highlight stays on the card while the long description belongs on the overview page."},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},r={render:()=>x.jsx(b,{})};var c,l,g;n.parameters={...n.parameters,docs:{...(c=n.parameters)==null?void 0:c.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(g=(l=n.parameters)==null?void 0:l.docs)==null?void 0:g.source}}};var d,m,p;o.parameters={...o.parameters,docs:{...(d=o.parameters)==null?void 0:d.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      highlight: '**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:\\n\\n• Event creation and management\\n• Meeting scheduling\\n• Reminder notifications\\n• Calendar sharing'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(p=(m=o.parameters)==null?void 0:m.docs)==null?void 0:p.source}}};var h,C,u;t.parameters={...t.parameters,docs:{...(h=t.parameters)==null?void 0:h.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: true
  }
}`,...(u=(C=t.parameters)==null?void 0:C.docs)==null?void 0:u.source}}};var f,k,v;a.parameters={...a.parameters,docs:{...(f=a.parameters)==null?void 0:f.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      description: 'This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.',
      highlight: 'Short marketplace highlight stays on the card while the long description belongs on the overview page.'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(v=(k=a.parameters)==null?void 0:k.docs)==null?void 0:v.source}}};var D,$,w;r.parameters={...r.parameters,docs:{...(D=r.parameters)==null?void 0:D.docs,source:{originalSource:`{
  render: () => <ConnectorCardSkeleton />
}`,...(w=($=r.parameters)==null?void 0:$.docs)==null?void 0:w.source}}};const U=["Default","WithHighlight","Connecting","LongDescription","Skeleton"];export{t as Connecting,n as Default,a as LongDescription,r as Skeleton,o as WithHighlight,U as __namedExportsOrder,L as default};

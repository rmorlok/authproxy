import{j as L}from"./jsx-runtime-BjG_zV1W.js";import{C as E,a as R}from"./ConnectorCard-BXQFp9YW.js";import"./client-6EFRD0gB.js";import{C as U}from"./index-DxM3CGmP.js";import"./theme-CKuQCuBO.js";import"./createSimplePaletteValueFilter-CvlNngyw.js";import"./index-yIsmwZOr.js";import"./ConnectorLogo-D_IJLdbp.js";import"./Box-CWJIOfo9.js";import"./Typography-USnMzuFJ.js";import"./Button-qoqN5xvQ.js";const _=(e,l)=>{const I=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${l}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(I)}`},j=e=>{const l=`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="120" viewBox="0 0 640 120" role="img" aria-label="${e} logo"><rect width="640" height="120" rx="18" fill="#111827"/><circle cx="66" cy="60" r="34" fill="#34d399"/><text x="118" y="66" fill="#f9fafb" font-family="Inter, Arial, sans-serif" font-size="44" font-weight="800">Wide Format Systems</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(l)}`},P={title:"Components/ConnectorCard",component:E,parameters:{layout:"centered"},tags:["autodocs"]},o={id:"google-calendar",version:1,state:U.ACTIVE,type:"oauth",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",highlight:"Manage events and appointments from Google Calendar.",logo:_("Google Calendar","#1a73e8")},n={args:{connector:o,onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},t={args:{connector:{...o,highlight:`**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:

• Event creation and management
• Meeting scheduling
• Reminder notifications
• Calendar sharing`},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},i={args:{connector:o,onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!0}},a={args:{connector:{...o,description:"This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.",highlight:"Short marketplace highlight stays on the card while the long description belongs on the overview page."},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},s={args:{connector:{...o,highlight:void 0,description:"Allow the agent to manage your calendar on your behalf. It is like having your own personal assistant."},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},r={args:{connector:{...o,display_name:"Wide Format Systems",highlight:"A wide logo should scale down inside the card without being cut off.",logo:j("Wide Format Systems")},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},c={render:()=>L.jsx(R,{})};var d,g,m;n.parameters={...n.parameters,docs:{...(d=n.parameters)==null?void 0:d.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(m=(g=n.parameters)==null?void 0:g.docs)==null?void 0:m.source}}};var h,p,C;t.parameters={...t.parameters,docs:{...(h=t.parameters)==null?void 0:h.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      highlight: '**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:\\n\\n• Event creation and management\\n• Meeting scheduling\\n• Reminder notifications\\n• Calendar sharing'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(C=(p=t.parameters)==null?void 0:p.docs)==null?void 0:C.source}}};var f,u,k;i.parameters={...i.parameters,docs:{...(f=i.parameters)==null?void 0:f.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: true
  }
}`,...(k=(u=i.parameters)==null?void 0:u.docs)==null?void 0:k.source}}};var w,D,v;a.parameters={...a.parameters,docs:{...(w=a.parameters)==null?void 0:w.docs,source:{originalSource:`{
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
}`,...(v=(D=a.parameters)==null?void 0:D.docs)==null?void 0:v.source}}};var y,$,x;s.parameters={...s.parameters,docs:{...(y=s.parameters)==null?void 0:y.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      highlight: undefined,
      description: 'Allow the agent to manage your calendar on your behalf. It is like having your own personal assistant.'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(x=($=s.parameters)==null?void 0:$.docs)==null?void 0:x.source}}};var S,b,F;r.parameters={...r.parameters,docs:{...(S=r.parameters)==null?void 0:S.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      display_name: 'Wide Format Systems',
      highlight: 'A wide logo should scale down inside the card without being cut off.',
      logo: wideLogoDataUri('Wide Format Systems')
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(F=(b=r.parameters)==null?void 0:b.docs)==null?void 0:F.source}}};var G,W,A;c.parameters={...c.parameters,docs:{...(G=c.parameters)==null?void 0:G.docs,source:{originalSource:`{
  render: () => <ConnectorCardSkeleton />
}`,...(A=(W=c.parameters)==null?void 0:W.docs)==null?void 0:A.source}}};const Q=["Default","WithHighlight","Connecting","LongDescription","DescriptionFallback","WideLogo","Skeleton"];export{i as Connecting,n as Default,s as DescriptionFallback,a as LongDescription,c as Skeleton,r as WideLogo,t as WithHighlight,Q as __namedExportsOrder,P as default};

import{j as L}from"./jsx-runtime-BjG_zV1W.js";import{C as R,a as U}from"./ConnectorCard-74VokUFw.js";import"./client-BIbib-UV.js";import{C as d}from"./index-C2XWl3Gq.js";import"./theme-BsVliGDX.js";import"./createSimplePaletteValueFilter-cJOvZn4l.js";import"./index-yIsmwZOr.js";import"./ConnectorLogo-CVdltLkv.js";import"./Box-CZbgkUlr.js";import"./Typography-Bzksp_jr.js";import"./Button-C8SQhaag.js";const _=(e,l)=>{const E=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${l}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(E)}`},T=e=>{const l=`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="120" viewBox="0 0 640 120" role="img" aria-label="${e} logo"><rect width="640" height="120" rx="18" fill="#111827"/><circle cx="66" cy="60" r="34" fill="#34d399"/><text x="118" y="66" fill="#f9fafb" font-family="Inter, Arial, sans-serif" font-size="44" font-weight="800">Wide Format Systems</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(l)}`},P={title:"Components/ConnectorCard",component:R,parameters:{layout:"centered"},tags:["autodocs"]},o={id:"google-calendar",version:1,state:d.ACTIVE,type:"oauth",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",highlight:"Manage events and appointments from Google Calendar.",logo:_("Google Calendar","#1a73e8"),versions:1,states:[d.ACTIVE]},n={args:{connector:o,onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},t={args:{connector:{...o,highlight:`**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:

• Event creation and management
• Meeting scheduling
• Reminder notifications
• Calendar sharing`},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},i={args:{connector:o,onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!0}},s={args:{connector:{...o,description:"This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.",highlight:"Short marketplace highlight stays on the card while the long description belongs on the overview page."},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},a={args:{connector:{...o,highlight:void 0,description:"Allow the agent to manage your calendar on your behalf. It is like having your own personal assistant."},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},r={args:{connector:{...o,display_name:"Wide Format Systems",highlight:"A wide logo should scale down inside the card without being cut off.",logo:T("Wide Format Systems")},onConnect:e=>console.log(`Connect clicked for ${e}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},c={render:()=>L.jsx(U,{})};var g,m,h;n.parameters={...n.parameters,docs:{...(g=n.parameters)==null?void 0:g.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(h=(m=n.parameters)==null?void 0:m.docs)==null?void 0:h.source}}};var p,C,f;t.parameters={...t.parameters,docs:{...(p=t.parameters)==null?void 0:p.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      highlight: '**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:\\n\\n• Event creation and management\\n• Meeting scheduling\\n• Reminder notifications\\n• Calendar sharing'
    },
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(f=(C=t.parameters)==null?void 0:C.docs)==null?void 0:f.source}}};var u,k,w;i.parameters={...i.parameters,docs:{...(u=i.parameters)==null?void 0:u.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: id => console.log(\`Connect clicked for \${id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: true
  }
}`,...(w=(k=i.parameters)==null?void 0:k.docs)==null?void 0:w.source}}};var v,D,y;s.parameters={...s.parameters,docs:{...(v=s.parameters)==null?void 0:v.docs,source:{originalSource:`{
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
}`,...(y=(D=s.parameters)==null?void 0:D.docs)==null?void 0:y.source}}};var $,x,S;a.parameters={...a.parameters,docs:{...($=a.parameters)==null?void 0:$.docs,source:{originalSource:`{
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
}`,...(S=(x=a.parameters)==null?void 0:x.docs)==null?void 0:S.source}}};var b,F,G;r.parameters={...r.parameters,docs:{...(b=r.parameters)==null?void 0:b.docs,source:{originalSource:`{
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
}`,...(G=(F=r.parameters)==null?void 0:F.docs)==null?void 0:G.source}}};var W,A,I;c.parameters={...c.parameters,docs:{...(W=c.parameters)==null?void 0:W.docs,source:{originalSource:`{
  render: () => <ConnectorCardSkeleton />
}`,...(I=(A=c.parameters)==null?void 0:A.docs)==null?void 0:I.source}}};const Q=["Default","WithHighlight","Connecting","LongDescription","DescriptionFallback","WideLogo","Skeleton"];export{i as Connecting,n as Default,a as DescriptionFallback,s as LongDescription,c as Skeleton,r as WideLogo,t as WithHighlight,Q as __namedExportsOrder,P as default};

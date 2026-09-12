import{j as R}from"./jsx-runtime-BjG_zV1W.js";import{C as E,a as j}from"./ConnectorCard-CTKEgb1w.js";import{c as l}from"./resources-D1r1Wyr4.js";import"./theme-C544Z3EA.js";import"./createSimplePaletteValueFilter-CDpnWFvD.js";import"./index-yIsmwZOr.js";import"./connections-CPVoMvMW.js";import"./Button-BqFH1kjI.js";import"./Typography-D32yZ0gM.js";import"./Box-DuDZjhmh.js";import"./CardActions-CK4gHLtU.js";const g=(e,d)=>{const L=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${d}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(L)}`},M=e=>{const d=`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="120" viewBox="0 0 640 120" role="img" aria-label="${e} logo"><rect width="640" height="120" rx="18" fill="#111827"/><circle cx="66" cy="60" r="34" fill="#34d399"/><text x="118" y="66" fill="#f9fafb" font-family="Inter, Arial, sans-serif" font-size="44" font-weight="800">Wide Format Systems</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(d)}`},Q={title:"Components/ConnectorCard",component:E,parameters:{layout:"centered"},tags:["autodocs"]},o=l({displayName:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",highlight:"Manage events and appointments from Google Calendar.",logo:{publicUrl:g("Google Calendar","#1a73e8")}}),n={args:{connector:o,onConnect:e=>console.log(`Connect clicked for ${e.metadata.id}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},t={args:{connector:{...o,spec:{...o.spec,definition:{...o.spec.definition,highlight:`**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:

• Event creation and management
• Meeting scheduling
• Reminder notifications
• Calendar sharing`}}},onConnect:e=>console.log(`Connect clicked for ${e.metadata.id}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},a={args:{connector:o,onConnect:e=>console.log(`Connect clicked for ${e.metadata.id}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!0}},i={args:{connector:l({description:"This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.",highlight:"Short marketplace highlight stays on the card while the long description belongs on the overview page.",logo:{publicUrl:g("Google Calendar","#1a73e8")}}),onConnect:e=>console.log(`Connect clicked for ${e.metadata.id}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},c={args:{connector:l({description:"Allow the agent to manage your calendar on your behalf. It is like having your own personal assistant.",logo:{publicUrl:g("Google Calendar","#1a73e8")}}),onConnect:e=>console.log(`Connect clicked for ${e.metadata.id}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},r={args:{connector:l({id:"wide-format-systems",displayName:"Wide Format Systems",highlight:"A wide logo should scale down inside the card without being cut off.",logo:{publicUrl:M("Wide Format Systems")}}),onConnect:e=>console.log(`Connect clicked for ${e.metadata.id}`),onDetails:e=>console.log(`Details clicked for ${e}`),isConnecting:!1}},s={render:()=>R.jsx(j,{})};var m,p,h;n.parameters={...n.parameters,docs:{...(m=n.parameters)==null?void 0:m.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: connector => console.log(\`Connect clicked for \${connector.metadata.id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(h=(p=n.parameters)==null?void 0:p.docs)==null?void 0:h.source}}};var f,u,C;t.parameters={...t.parameters,docs:{...(f=t.parameters)==null?void 0:f.docs,source:{originalSource:`{
  args: {
    connector: {
      ...mockConnector,
      spec: {
        ...mockConnector.spec,
        definition: {
          ...mockConnector.spec.definition,
          highlight: '**Sync your calendar** with Google Calendar to manage events, appointments, and meetings. Features include:\\n\\n• Event creation and management\\n• Meeting scheduling\\n• Reminder notifications\\n• Calendar sharing'
        }
      }
    },
    onConnect: connector => console.log(\`Connect clicked for \${connector.metadata.id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(C=(u=t.parameters)==null?void 0:u.docs)==null?void 0:C.source}}};var k,w,D;a.parameters={...a.parameters,docs:{...(k=a.parameters)==null?void 0:k.docs,source:{originalSource:`{
  args: {
    connector: mockConnector,
    onConnect: connector => console.log(\`Connect clicked for \${connector.metadata.id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: true
  }
}`,...(D=(w=a.parameters)==null?void 0:w.docs)==null?void 0:D.source}}};var y,v,$;i.parameters={...i.parameters,docs:{...(y=i.parameters)==null?void 0:y.docs,source:{originalSource:`{
  args: {
    connector: connectorFixture({
      description: 'This is a very long description that should wrap to multiple lines. Connect to your Google Calendar to manage events and appointments, schedule meetings, and get reminders about upcoming events.',
      highlight: 'Short marketplace highlight stays on the card while the long description belongs on the overview page.',
      logo: {
        publicUrl: logoDataUri('Google Calendar', '#1a73e8')
      }
    }),
    onConnect: connector => console.log(\`Connect clicked for \${connector.metadata.id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...($=(v=i.parameters)==null?void 0:v.docs)==null?void 0:$.source}}};var x,b,S;c.parameters={...c.parameters,docs:{...(x=c.parameters)==null?void 0:x.docs,source:{originalSource:`{
  args: {
    connector: connectorFixture({
      description: 'Allow the agent to manage your calendar on your behalf. It is like having your own personal assistant.',
      logo: {
        publicUrl: logoDataUri('Google Calendar', '#1a73e8')
      }
    }),
    onConnect: connector => console.log(\`Connect clicked for \${connector.metadata.id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(S=(b=c.parameters)==null?void 0:b.docs)==null?void 0:S.source}}};var U,F,G;r.parameters={...r.parameters,docs:{...(U=r.parameters)==null?void 0:U.docs,source:{originalSource:`{
  args: {
    connector: connectorFixture({
      id: 'wide-format-systems',
      displayName: 'Wide Format Systems',
      highlight: 'A wide logo should scale down inside the card without being cut off.',
      logo: {
        publicUrl: wideLogoDataUri('Wide Format Systems')
      }
    }),
    onConnect: connector => console.log(\`Connect clicked for \${connector.metadata.id}\`),
    onDetails: id => console.log(\`Details clicked for \${id}\`),
    isConnecting: false
  }
}`,...(G=(F=r.parameters)==null?void 0:F.docs)==null?void 0:G.source}}};var W,A,I;s.parameters={...s.parameters,docs:{...(W=s.parameters)==null?void 0:W.docs,source:{originalSource:`{
  render: () => <ConnectorCardSkeleton />
}`,...(I=(A=s.parameters)==null?void 0:A.docs)==null?void 0:I.source}}};const V=["Default","WithHighlight","Connecting","LongDescription","DescriptionFallback","WideLogo","Skeleton"];export{a as Connecting,n as Default,c as DescriptionFallback,i as LongDescription,s as Skeleton,r as WideLogo,t as WithHighlight,V as __namedExportsOrder,Q as default};

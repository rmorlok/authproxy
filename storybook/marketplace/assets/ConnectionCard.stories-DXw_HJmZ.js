import{j as r}from"./jsx-runtime-BjG_zV1W.js";import{C as an,a as sn}from"./ConnectionCard-Cm6KZdsy.js";import"./client-6EFRD0gB.js";import{C as dn}from"./index-BhpGBZnY.js";import{C as nn,a as o}from"./connections-DMQz4Kw3.js";import{P as en,c as on,a as tn,b as rn}from"./connectionPresentation-C_q4Noyw.js";import"./index-yIsmwZOr.js";import"./createSvgIcon-ByfhJewD.js";import"./createSimplePaletteValueFilter-CvlNngyw.js";import"./theme-CKuQCuBO.js";import"./ConnectorLogo-P4ORXqmW.js";import"./Box-CWJIOfo9.js";import"./Typography-USnMzuFJ.js";import"./index-BXrwOJ9g.js";import"./IconButton-T-YZe696.js";import"./Button-qoqN5xvQ.js";import"./Chip-BT1MOgdz.js";import"./index-M3uX8AIl.js";const ln=(e,t)=>{const cn=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${t}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(cn)}`},mn=e=>{const t=`<svg xmlns="http://www.w3.org/2000/svg" width="640" height="120" viewBox="0 0 640 120" role="img" aria-label="${e} logo"><rect width="640" height="120" rx="18" fill="#111827"/><circle cx="66" cy="60" r="34" fill="#34d399"/><text x="118" y="66" fill="#f9fafb" font-family="Inter, Arial, sans-serif" font-size="44" font-weight="800">Wide Format Systems</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(t)}`},S=ln("Google Calendar","#1a73e8"),gn=mn("Wide Format Systems"),un=on({reducer:{connectors:rn,connections:tn},preloadedState:{connectors:{items:[{id:"google-calendar",displayName:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:S}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null}}}),Ln={title:"Components/ConnectionCard",component:an,parameters:{layout:"centered"},tags:["autodocs"],decorators:[e=>r.jsx(en,{store:un,children:r.jsx(e,{})})]},n={id:"123e4567-e89b-12d3-a456-426614174000",connector:{type:"google-calendar",id:"923e4567-e89b-12d3-a456-426614174009",version:0,state:dn.PRIMARY,displayName:"Google Calendar",description:"A google calendar connector",logo:S},state:o.CONFIGURED,healthState:nn.HEALTHY,createdAt:"2023-04-01T12:00:00Z",updatedAt:"2023-04-01T12:00:00Z"},c={args:{connection:n}},a={args:{connection:n,highlightNew:!0}},s={args:{connection:{...n,connector:{...n.connector,hasConfigure:!0}}}},i={args:{connection:{...n,connector:{...n.connector,displayName:"Wide Format Systems",highlight:"A wide logo should scale down inside the header without being cut off.",logo:gn}}}},d={args:{connection:{...n,connector:{...n.connector,hasConfigure:!0},healthState:nn.UNHEALTHY}}},l={args:{connection:{...n,state:o.SETUP}}},m={args:{connection:{...n,state:o.DISABLED}}},g={args:{connection:{...n,state:o.DISCONNECTING}}},u={args:{connection:{...n,state:o.DISCONNECTED}}},p={args:{connection:{...n,connectorId:"unknown-connector"}}},C={args:{connection:{...n,state:o.DISCONNECTING}},decorators:[e=>{const t=on({reducer:{connectors:rn,connections:tn},preloadedState:{connectors:{items:[{id:"google-calendar",displayName:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:S}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!0,disconnectionError:null,currentTaskId:"task-123"}}});return r.jsx(en,{store:t,children:r.jsx(e,{})})}]},h={render:()=>r.jsx(sn,{})};var f,w,k;c.parameters={...c.parameters,docs:{...(f=c.parameters)==null?void 0:f.docs,source:{originalSource:`{
  args: {
    connection: mockConnection
  }
}`,...(k=(w=c.parameters)==null?void 0:w.docs)==null?void 0:k.source}}};var N,y,x;a.parameters={...a.parameters,docs:{...(N=a.parameters)==null?void 0:N.docs,source:{originalSource:`{
  args: {
    connection: mockConnection,
    highlightNew: true
  }
}`,...(x=(y=a.parameters)==null?void 0:y.docs)==null?void 0:x.source}}};var I,E,v;s.parameters={...s.parameters,docs:{...(I=s.parameters)==null?void 0:I.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        hasConfigure: true
      }
    }
  }
}`,...(v=(E=s.parameters)==null?void 0:E.docs)==null?void 0:v.source}}};var D,T,G;i.parameters={...i.parameters,docs:{...(D=i.parameters)==null?void 0:D.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        displayName: 'Wide Format Systems',
        highlight: 'A wide logo should scale down inside the header without being cut off.',
        logo: wideFormatLogo
      }
    }
  }
}`,...(G=(T=i.parameters)==null?void 0:T.docs)==null?void 0:G.source}}};var A,U,L;d.parameters={...d.parameters,docs:{...(A=d.parameters)==null?void 0:A.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        hasConfigure: true
      },
      healthState: ConnectionHealthState.UNHEALTHY
    }
  }
}`,...(L=(U=d.parameters)==null?void 0:U.docs)==null?void 0:L.source}}};var b,R,F;l.parameters={...l.parameters,docs:{...(b=l.parameters)==null?void 0:b.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.SETUP
    }
  }
}`,...(F=(R=l.parameters)==null?void 0:R.docs)==null?void 0:F.source}}};var P,H,O;m.parameters={...m.parameters,docs:{...(P=m.parameters)==null?void 0:P.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISABLED
    }
  }
}`,...(O=(H=m.parameters)==null?void 0:H.docs)==null?void 0:O.source}}};var W,j,$;g.parameters={...g.parameters,docs:{...(W=g.parameters)==null?void 0:W.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTING
    }
  }
}`,...($=(j=g.parameters)==null?void 0:j.docs)==null?void 0:$.source}}};var B,Y,_;u.parameters={...u.parameters,docs:{...(B=u.parameters)==null?void 0:B.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTED
    }
  }
}`,...(_=(Y=u.parameters)==null?void 0:Y.docs)==null?void 0:_.source}}};var z,Z,M;p.parameters={...p.parameters,docs:{...(z=p.parameters)==null?void 0:z.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connectorId: 'unknown-connector'
    }
  }
}`,...(M=(Z=p.parameters)==null?void 0:Z.docs)==null?void 0:M.source}}};var V,q,J;C.parameters={...C.parameters,docs:{...(V=C.parameters)==null?void 0:V.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTING
    }
  },
  decorators: [Story => {
    const store = configureStore({
      reducer: {
        connectors: connectorsReducer,
        connections: connectionsReducer
      },
      preloadedState: {
        connectors: {
          items: [{
            id: 'google-calendar',
            displayName: 'Google Calendar',
            description: 'Connect to your Google Calendar to manage events and appointments.',
            logo: googleCalendarLogo
          }],
          status: 'succeeded',
          error: null
        },
        connections: {
          items: [],
          status: 'idle',
          error: null,
          initiatingConnection: false,
          initiationError: null,
          disconnectingConnection: true,
          disconnectionError: null,
          currentTaskId: 'task-123'
        }
      }
    });
    return <Provider store={store}>
          <Story />
        </Provider>;
  }]
}`,...(J=(q=C.parameters)==null?void 0:q.docs)==null?void 0:J.source}}};var K,Q,X;h.parameters={...h.parameters,docs:{...(K=h.parameters)==null?void 0:K.docs,source:{originalSource:`{
  render: () => <ConnectionCardSkeleton />
}`,...(X=(Q=h.parameters)==null?void 0:Q.docs)==null?void 0:X.source}}};const bn=["Connected","NewlyConnected","ConnectedConfigurable","WideLogo","Unhealthy","Created","Failed","Disconnecting","Disconnected","UnknownConnector","WithTaskInProgress","Skeleton"];export{c as Connected,s as ConnectedConfigurable,l as Created,u as Disconnected,g as Disconnecting,m as Failed,a as NewlyConnected,h as Skeleton,d as Unhealthy,p as UnknownConnector,i as WideLogo,C as WithTaskInProgress,bn as __namedExportsOrder,Ln as default};

import{j as t}from"./jsx-runtime-BjG_zV1W.js";import{C as X,P as z,c as V,a as nn,b as q,d as J}from"./ConnectionCard-DZmlsVdF.js";import"./client-0PMYh2xf.js";import{C as S}from"./index-BcFHX6i3.js";import{C as K,a as e}from"./connections-DEdR_kbc.js";import"./index-yIsmwZOr.js";import"./useSlot-CvJKuwFf.js";import"./createSimplePaletteValueFilter-DRANIdZ4.js";import"./Button-DiNVVHLo.js";import"./Typography-zR6YQtWt.js";import"./Box-qKFn2-pG.js";import"./theme-D4YlWSlu.js";import"./IconButton-VjDeSjWN.js";import"./Chip-NDjmWq1o.js";import"./index-M3uX8AIl.js";const en=(o,p)=>{const Q=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${o} logo"><rect width="280" height="140" rx="8" fill="${p}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="#fff" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">GC</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(Q)}`},C=en("Google Calendar","#1a73e8"),on=V({reducer:{connectors:J,connections:q},preloadedState:{connectors:{items:[{id:"google-calendar",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:C}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null}}}),kn={title:"Components/ConnectionCard",component:X,parameters:{layout:"centered"},tags:["autodocs"],decorators:[o=>t.jsx(z,{store:on,children:t.jsx(o,{})})]},n={id:"123e4567-e89b-12d3-a456-426614174000",connector:{type:"google-calendar",versions:0,states:[S.PRIMARY],id:"923e4567-e89b-12d3-a456-426614174009",version:0,state:S.PRIMARY,display_name:"Google Calendar",description:"A google calendar connector",logo:C},state:e.CONFIGURED,health_state:K.HEALTHY,created_at:"2023-04-01T12:00:00Z",updated_at:"2023-04-01T12:00:00Z"},r={args:{connection:n}},c={args:{connection:{...n,connector:{...n.connector,has_configure:!0}}}},a={args:{connection:{...n,connector:{...n.connector,has_configure:!0},health_state:K.UNHEALTHY}}},s={args:{connection:{...n,state:e.SETUP}}},i={args:{connection:{...n,state:e.DISABLED}}},d={args:{connection:{...n,state:e.DISCONNECTING}}},l={args:{connection:{...n,state:e.DISCONNECTED}}},m={args:{connection:{...n,connector_id:"unknown-connector"}}},g={args:{connection:{...n,state:e.DISCONNECTING}},decorators:[o=>{const p=V({reducer:{connectors:J,connections:q},preloadedState:{connectors:{items:[{id:"google-calendar",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:C}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!0,disconnectionError:null,currentTaskId:"task-123"}}});return t.jsx(z,{store:p,children:t.jsx(o,{})})}]},u={render:()=>t.jsx(nn,{})};var h,f,k;r.parameters={...r.parameters,docs:{...(h=r.parameters)==null?void 0:h.docs,source:{originalSource:`{
  args: {
    connection: mockConnection
  }
}`,...(k=(f=r.parameters)==null?void 0:f.docs)==null?void 0:k.source}}};var E,I,N;c.parameters={...c.parameters,docs:{...(E=c.parameters)==null?void 0:E.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        has_configure: true
      }
    }
  }
}`,...(N=(I=c.parameters)==null?void 0:I.docs)==null?void 0:N.source}}};var D,T,_;a.parameters={...a.parameters,docs:{...(D=a.parameters)==null?void 0:D.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector: {
        ...mockConnection.connector,
        has_configure: true
      },
      health_state: ConnectionHealthState.UNHEALTHY
    }
  }
}`,...(_=(T=a.parameters)==null?void 0:T.docs)==null?void 0:_.source}}};var x,y,v;s.parameters={...s.parameters,docs:{...(x=s.parameters)==null?void 0:x.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.SETUP
    }
  }
}`,...(v=(y=s.parameters)==null?void 0:y.docs)==null?void 0:v.source}}};var G,w,R;i.parameters={...i.parameters,docs:{...(G=i.parameters)==null?void 0:G.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISABLED
    }
  }
}`,...(R=(w=i.parameters)==null?void 0:w.docs)==null?void 0:R.source}}};var U,P,A;d.parameters={...d.parameters,docs:{...(U=d.parameters)==null?void 0:U.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTING
    }
  }
}`,...(A=(P=d.parameters)==null?void 0:P.docs)==null?void 0:A.source}}};var H,O,b;l.parameters={...l.parameters,docs:{...(H=l.parameters)==null?void 0:H.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTED
    }
  }
}`,...(b=(O=l.parameters)==null?void 0:O.docs)==null?void 0:b.source}}};var j,L,Y;m.parameters={...m.parameters,docs:{...(j=m.parameters)==null?void 0:j.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector_id: 'unknown-connector'
    }
  }
}`,...(Y=(L=m.parameters)==null?void 0:L.docs)==null?void 0:Y.source}}};var B,F,$;g.parameters={...g.parameters,docs:{...(B=g.parameters)==null?void 0:B.docs,source:{originalSource:`{
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
            display_name: 'Google Calendar',
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
}`,...($=(F=g.parameters)==null?void 0:F.docs)==null?void 0:$.source}}};var M,W,Z;u.parameters={...u.parameters,docs:{...(M=u.parameters)==null?void 0:M.docs,source:{originalSource:`{
  render: () => <ConnectionCardSkeleton />
}`,...(Z=(W=u.parameters)==null?void 0:W.docs)==null?void 0:Z.source}}};const En=["Connected","ConnectedConfigurable","Unhealthy","Created","Failed","Disconnecting","Disconnected","UnknownConnector","WithTaskInProgress","Skeleton"];export{r as Connected,c as ConnectedConfigurable,s as Created,l as Disconnected,d as Disconnecting,i as Failed,u as Skeleton,a as Unhealthy,m as UnknownConnector,g as WithTaskInProgress,En as __namedExportsOrder,kn as default};

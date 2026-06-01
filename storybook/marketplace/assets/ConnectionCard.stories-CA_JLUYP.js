import{j as o}from"./jsx-runtime-BjG_zV1W.js";import{C as q,P as F,c as B,a as z,b as M,d as W}from"./ConnectionCard-BQwOvvJ4.js";import"./client-HmSzpqVp.js";import{C as u}from"./index-Cck04WQ2.js";import{C as Z,a as e}from"./connections-P_7JUaGq.js";import"./index-yIsmwZOr.js";import"./useSlot-CquYDC0W.js";import"./createSimplePaletteValueFilter-jJDq419X.js";import"./Button-C_mwJF14.js";import"./Typography-Cb-G69Z7.js";import"./Box-Rx5U04Ep.js";import"./index-M3uX8AIl.js";const J=B({reducer:{connectors:W,connections:M},preloadedState:{connectors:{items:[{id:"google-calendar",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:"https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg"}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null}}}),dn={title:"Components/ConnectionCard",component:q,parameters:{layout:"centered"},tags:["autodocs"],decorators:[p=>o.jsx(F,{store:J,children:o.jsx(p,{})})]},n={id:"123e4567-e89b-12d3-a456-426614174000",connector:{type:"google-calendar",versions:0,states:[u.PRIMARY],id:"923e4567-e89b-12d3-a456-426614174009",version:0,state:u.PRIMARY,display_name:"Google Calendar",description:"A google calendar connector",logo:"https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg"},state:e.CONFIGURED,health_state:Z.HEALTHY,created_at:"2023-04-01T12:00:00Z",updated_at:"2023-04-01T12:00:00Z"},t={args:{connection:n}},r={args:{connection:{...n,health_state:Z.UNHEALTHY}}},a={args:{connection:{...n,state:e.SETUP}}},c={args:{connection:{...n,state:e.DISABLED}}},s={args:{connection:{...n,state:e.DISCONNECTING}}},i={args:{connection:{...n,state:e.DISCONNECTED}}},d={args:{connection:{...n,connector_id:"unknown-connector"}}},l={args:{connection:{...n,state:e.DISCONNECTING}},decorators:[p=>{const V=B({reducer:{connectors:W,connections:M},preloadedState:{connectors:{items:[{id:"google-calendar",display_name:"Google Calendar",description:"Connect to your Google Calendar to manage events and appointments.",logo:"https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg"}],status:"succeeded",error:null},connections:{items:[],status:"idle",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!0,disconnectionError:null,currentTaskId:"task-123"}}});return o.jsx(F,{store:V,children:o.jsx(p,{})})}]},m={render:()=>o.jsx(z,{})};var g,C,S;t.parameters={...t.parameters,docs:{...(g=t.parameters)==null?void 0:g.docs,source:{originalSource:`{
  args: {
    connection: mockConnection
  }
}`,...(S=(C=t.parameters)==null?void 0:C.docs)==null?void 0:S.source}}};var k,_,E;r.parameters={...r.parameters,docs:{...(k=r.parameters)==null?void 0:k.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      health_state: ConnectionHealthState.UNHEALTHY
    }
  }
}`,...(E=(_=r.parameters)==null?void 0:_.docs)==null?void 0:E.source}}};var h,I,N;a.parameters={...a.parameters,docs:{...(h=a.parameters)==null?void 0:h.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.SETUP
    }
  }
}`,...(N=(I=a.parameters)==null?void 0:I.docs)==null?void 0:N.source}}};var T,D,G;c.parameters={...c.parameters,docs:{...(T=c.parameters)==null?void 0:T.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISABLED
    }
  }
}`,...(G=(D=c.parameters)==null?void 0:D.docs)==null?void 0:G.source}}};var y,f,v;s.parameters={...s.parameters,docs:{...(y=s.parameters)==null?void 0:y.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTING
    }
  }
}`,...(v=(f=s.parameters)==null?void 0:f.docs)==null?void 0:v.source}}};var w,P,R;i.parameters={...i.parameters,docs:{...(w=i.parameters)==null?void 0:w.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      state: ConnectionState.DISCONNECTED
    }
  }
}`,...(R=(P=i.parameters)==null?void 0:P.docs)==null?void 0:R.source}}};var x,U,A;d.parameters={...d.parameters,docs:{...(x=d.parameters)==null?void 0:x.docs,source:{originalSource:`{
  args: {
    connection: {
      ...mockConnection,
      connector_id: 'unknown-connector'
    }
  }
}`,...(A=(U=d.parameters)==null?void 0:U.docs)==null?void 0:A.source}}};var H,O,j;l.parameters={...l.parameters,docs:{...(H=l.parameters)==null?void 0:H.docs,source:{originalSource:`{
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
            logo: 'https://upload.wikimedia.org/wikipedia/commons/a/a5/Google_Calendar_icon_%282020%29.svg'
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
}`,...(j=(O=l.parameters)==null?void 0:O.docs)==null?void 0:j.source}}};var L,Y,b;m.parameters={...m.parameters,docs:{...(L=m.parameters)==null?void 0:L.docs,source:{originalSource:`{
  render: () => <ConnectionCardSkeleton />
}`,...(b=(Y=m.parameters)==null?void 0:Y.docs)==null?void 0:b.source}}};const ln=["Connected","Unhealthy","Created","Failed","Disconnecting","Disconnected","UnknownConnector","WithTaskInProgress","Skeleton"];export{t as Connected,a as Created,i as Disconnected,s as Disconnecting,c as Failed,m as Skeleton,r as Unhealthy,d as UnknownConnector,l as WithTaskInProgress,ln as __namedExportsOrder,dn as default};

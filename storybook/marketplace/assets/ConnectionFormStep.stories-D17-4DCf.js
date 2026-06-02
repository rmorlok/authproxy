import{C as S}from"./ConnectionFormStep-DtUj0uS6.js";import{f as o}from"./index-Droui3yq.js";import"./jsx-runtime-BjG_zV1W.js";import"./index-yIsmwZOr.js";import"./createSimplePaletteValueFilter-CffhNcpo.js";import"./connections-Cnm8TFfr.js";import"./IconButton-CRpx-Pp7.js";import"./Button-BU-5bqsh.js";import"./Typography-Bi0mUh4H.js";import"./Box-Dxb9LzqI.js";import"./index-M3uX8AIl.js";import"./client-CitdaXMW.js";import"./Close-mSrv2z3g.js";const K={title:"Components/ConnectionFormStep",component:S,parameters:{layout:"centered"},tags:["autodocs"],args:{onSubmit:o(),onCancel:o(),isSubmitting:!1,connectionId:"cxn_test550e8400abcde"}},e={args:{jsonSchema:{type:"object",properties:{apiKey:{type:"string",title:"API Key",description:"Your API key for the service"},region:{type:"string",title:"Region",enum:["us-east-1","us-west-2","eu-west-1","ap-southeast-1"]}},required:["apiKey","region"]},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/apiKey"},{type:"Control",scope:"#/properties/region"}]}}},t={args:{jsonSchema:{type:"object",properties:{name:{type:"string",title:"Connection Name",minLength:3,maxLength:50},environment:{type:"string",title:"Environment",enum:["production","staging","development"],default:"development"},credentials:{type:"object",title:"Credentials",properties:{username:{type:"string",title:"Username"},password:{type:"string",title:"Password"}},required:["username","password"]},enableLogging:{type:"boolean",title:"Enable Request Logging",default:!0},maxRetries:{type:"integer",title:"Max Retries",minimum:0,maximum:10,default:3}},required:["name","environment","credentials"]},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/name"},{type:"Control",scope:"#/properties/environment"},{type:"Group",label:"Authentication",elements:[{type:"Control",scope:"#/properties/credentials/properties/username"},{type:"Control",scope:"#/properties/credentials/properties/password",options:{format:"password"}}]},{type:"HorizontalLayout",elements:[{type:"Control",scope:"#/properties/enableLogging"},{type:"Control",scope:"#/properties/maxRetries"}]}]}}},n={args:{...e.args,isSubmitting:!0}},r={args:{jsonSchema:{type:"object",properties:{token:{type:"string",title:"Access Token",description:"Paste your personal access token here"}},required:["token"]},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/token"}]}}};var s,i,p;e.parameters={...e.parameters,docs:{...(s=e.parameters)==null?void 0:s.docs,source:{originalSource:`{
  args: {
    jsonSchema: {
      type: 'object',
      properties: {
        apiKey: {
          type: 'string',
          title: 'API Key',
          description: 'Your API key for the service'
        },
        region: {
          type: 'string',
          title: 'Region',
          enum: ['us-east-1', 'us-west-2', 'eu-west-1', 'ap-southeast-1']
        }
      },
      required: ['apiKey', 'region']
    },
    uiSchema: {
      type: 'VerticalLayout',
      elements: [{
        type: 'Control',
        scope: '#/properties/apiKey'
      }, {
        type: 'Control',
        scope: '#/properties/region'
      }]
    }
  }
}`,...(p=(i=e.parameters)==null?void 0:i.docs)==null?void 0:p.source}}};var a,m,c;t.parameters={...t.parameters,docs:{...(a=t.parameters)==null?void 0:a.docs,source:{originalSource:`{
  args: {
    jsonSchema: {
      type: 'object',
      properties: {
        name: {
          type: 'string',
          title: 'Connection Name',
          minLength: 3,
          maxLength: 50
        },
        environment: {
          type: 'string',
          title: 'Environment',
          enum: ['production', 'staging', 'development'],
          default: 'development'
        },
        credentials: {
          type: 'object',
          title: 'Credentials',
          properties: {
            username: {
              type: 'string',
              title: 'Username'
            },
            password: {
              type: 'string',
              title: 'Password'
            }
          },
          required: ['username', 'password']
        },
        enableLogging: {
          type: 'boolean',
          title: 'Enable Request Logging',
          default: true
        },
        maxRetries: {
          type: 'integer',
          title: 'Max Retries',
          minimum: 0,
          maximum: 10,
          default: 3
        }
      },
      required: ['name', 'environment', 'credentials']
    },
    uiSchema: {
      type: 'VerticalLayout',
      elements: [{
        type: 'Control',
        scope: '#/properties/name'
      }, {
        type: 'Control',
        scope: '#/properties/environment'
      }, {
        type: 'Group',
        label: 'Authentication',
        elements: [{
          type: 'Control',
          scope: '#/properties/credentials/properties/username'
        }, {
          type: 'Control',
          scope: '#/properties/credentials/properties/password',
          options: {
            format: 'password'
          }
        }]
      }, {
        type: 'HorizontalLayout',
        elements: [{
          type: 'Control',
          scope: '#/properties/enableLogging'
        }, {
          type: 'Control',
          scope: '#/properties/maxRetries'
        }]
      }]
    }
  }
}`,...(c=(m=t.parameters)==null?void 0:m.docs)==null?void 0:c.source}}};var l,u,y;n.parameters={...n.parameters,docs:{...(l=n.parameters)==null?void 0:l.docs,source:{originalSource:`{
  args: {
    ...SimpleTextForm.args,
    isSubmitting: true
  }
}`,...(y=(u=n.parameters)==null?void 0:u.docs)==null?void 0:y.source}}};var g,d,C;r.parameters={...r.parameters,docs:{...(g=r.parameters)==null?void 0:g.docs,source:{originalSource:`{
  args: {
    jsonSchema: {
      type: 'object',
      properties: {
        token: {
          type: 'string',
          title: 'Access Token',
          description: 'Paste your personal access token here'
        }
      },
      required: ['token']
    },
    uiSchema: {
      type: 'VerticalLayout',
      elements: [{
        type: 'Control',
        scope: '#/properties/token'
      }]
    }
  }
}`,...(C=(d=r.parameters)==null?void 0:d.docs)==null?void 0:C.source}}};const P=["SimpleTextForm","ComplexForm","Submitting","MinimalForm"];export{t as ComplexForm,r as MinimalForm,e as SimpleTextForm,n as Submitting,P as __namedExportsOrder,K as default};

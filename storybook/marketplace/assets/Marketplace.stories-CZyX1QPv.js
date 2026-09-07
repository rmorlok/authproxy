import{j as t}from"./jsx-runtime-BjG_zV1W.js";import{r as p}from"./index-yIsmwZOr.js";import{u as He,d as S,H as Co,I as yo,J as vo,K as So,L as jo,M as wo,N as ko,O as en,Q as Io,T as Eo,e as zt,g as $t,G as Bt,m as Ht,s as Ao,f as Po,h as Ro,C as Lo,i as To,j as Mo,k as No,U as Do,V as Fo,W as Oo,X as zo,l as U,o as nn,Y as $o,q as Bo,t as Ho,v as Uo,Z as Vo,F as Go,_ as _o,E as Wo,c as qo,$ as Yo,P as Ko,B as Xo,a0 as Jo,a as Qo,b as Zo,a1 as er}from"./connectionPresentation-j16F52-m.js";import{i as Le,b as tn,a as Me,C as Ue}from"./connections-DX4lTWui.js";import{m as w,c as nr}from"./theme-C544Z3EA.js";import{N as on,d as tr,b as or,a as rr,n as sr,c as V}from"./resources-Cuh74jUd.js";import{L as rn,T as ar,B as ir,D as cr,A as lr,E as Ut,G as M,a as dr}from"./ConnectionFormStep-vey3VLEF.js";import{L as ur,I as pr,C as mr,a as gr}from"./ConnectionCard-DiHSzgNj.js";import{c as Ve}from"./createSvgIcon-BzPbi2jQ.js";import{a as Vt,O as fr,R as hr,f as sn}from"./index-BXrwOJ9g.js";import{c as br,e as Ne,f as xr,g as an,P as Cr,b as ee,B as _}from"./Button-BqFH1kjI.js";import{g as Gt,u as yr,T as I}from"./Typography-D32yZ0gM.js";import{u as Pe,g as Ge,a as _e,s as z,b as _t,n as T,d as We,o as W,p as X,A as qe,B as Ye,K as cn,L as vr}from"./createSimplePaletteValueFilter-CDpnWFvD.js";import{f as Sr,o as Te,G as jr,e as ne,M as ln,D as dn,a as un,b as pn,d as wr}from"./MenuItem-BRDK3hIB.js";import{A as K,C as Ke}from"./Container-DBdCHXDN.js";import{B as P,C as De}from"./Box-DuDZjhmh.js";import{I as kr}from"./IconButton-BY81ztgX.js";import{b as Ir,L as Wt,A as Er,C as qt,u as Ar,c as Pr,d as Rr}from"./ConnectorDetail-CZn7jObp.js";import{C as Yt,a as Kt}from"./ConnectorCard-Cra5yMXu.js";import{u as Lr}from"./Chip-0IuoHKYd.js";import"./useThemeProps-CE5O-xsb.js";import"./Close-By8nJtaj.js";import"./Stack-Ds5OSx19.js";import"./CardActions-CK4gHLtU.js";import"./index-M3uX8AIl.js";function mn(e){return e.substring(2).toLowerCase()}function Tr(e,n){return n.documentElement.clientWidth<e.clientX||n.documentElement.clientHeight<e.clientY}function Mr(e){const{children:n,disableReactTree:o=!1,mouseEvent:r="onClick",onClickAway:c,touchEvent:l="onTouchEnd"}=e,u=p.useRef(!1),d=p.useRef(null),m=p.useRef(!1),x=p.useRef(!1);p.useEffect(()=>(setTimeout(()=>{m.current=!0},0),()=>{m.current=!1}),[]);const i=br(Sr(n),d),g=Ne(s=>{const f=x.current;x.current=!1;const k=Te(d.current);if(!m.current||!d.current||"clientX"in s&&Tr(s,k))return;if(u.current){u.current=!1;return}let b;s.composedPath?b=s.composedPath().includes(d.current):b=!k.documentElement.contains(s.target)||d.current.contains(s.target),!b&&(o||!f)&&c(s)}),A=s=>f=>{x.current=!0;const k=n.props[s];k&&k(f)},C={ref:i};return l!==!1&&(C[l]=A(l)),p.useEffect(()=>{if(l!==!1){const s=mn(l),f=Te(d.current),k=()=>{u.current=!0};return f.addEventListener(s,g),f.addEventListener("touchmove",k),()=>{f.removeEventListener(s,g),f.removeEventListener("touchmove",k)}}},[g,l]),r!==!1&&(C[r]=A(r)),p.useEffect(()=>{if(r!==!1){const s=mn(r),f=Te(d.current);return f.addEventListener(s,g),()=>{f.removeEventListener(s,g)}}},[g,r]),p.cloneElement(n,C)}const Fe=typeof Gt({})=="function",Nr=(e,n)=>({WebkitFontSmoothing:"antialiased",MozOsxFontSmoothing:"grayscale",boxSizing:"border-box",WebkitTextSizeAdjust:"100%",...n&&!e.vars&&{colorScheme:e.palette.mode}}),Dr=e=>({color:(e.vars||e).palette.text.primary,...e.typography.body1,backgroundColor:(e.vars||e).palette.background.default,"@media print":{backgroundColor:(e.vars||e).palette.common.white}}),Xt=(e,n=!1)=>{var l,u;const o={};n&&e.colorSchemes&&typeof e.getColorSchemeSelector=="function"&&Object.entries(e.colorSchemes).forEach(([d,m])=>{var i,g;const x=e.getColorSchemeSelector(d);x.startsWith("@")?o[x]={":root":{colorScheme:(i=m.palette)==null?void 0:i.mode}}:o[x.replace(/\s*&/,"")]={colorScheme:(g=m.palette)==null?void 0:g.mode}});let r={html:Nr(e,n),"*, *::before, *::after":{boxSizing:"inherit"},"strong, b":{fontWeight:e.typography.fontWeightBold},body:{margin:0,...Dr(e),"&::backdrop":{backgroundColor:(e.vars||e).palette.background.default}},...o};const c=(u=(l=e.components)==null?void 0:l.MuiCssBaseline)==null?void 0:u.styleOverrides;return c&&(r=[r,c]),r},Ae="mui-ecs",Fr=e=>{const n=Xt(e,!1),o=Array.isArray(n)?n[0]:n;return!e.vars&&o&&(o.html[`:root:has(${Ae})`]={colorScheme:e.palette.mode}),e.colorSchemes&&Object.entries(e.colorSchemes).forEach(([r,c])=>{var u,d;const l=e.getColorSchemeSelector(r);l.startsWith("@")?o[l]={[`:root:not(:has(.${Ae}))`]:{colorScheme:(u=c.palette)==null?void 0:u.mode}}:o[l.replace(/\s*&/,"")]={[`&:not(:has(.${Ae}))`]:{colorScheme:(d=c.palette)==null?void 0:d.mode}}}),n},Or=Gt(Fe?({theme:e,enableColorScheme:n})=>Xt(e,n):({theme:e})=>Fr(e));function zr(e){const n=Pe({props:e,name:"MuiCssBaseline"}),{children:o,enableColorScheme:r=!1}=n;return t.jsxs(p.Fragment,{children:[Fe&&t.jsx(Or,{enableColorScheme:r}),!Fe&&!r&&t.jsx("span",{className:Ae,style:{display:"none"}}),o]})}function $r(e){return Ge("MuiLinearProgress",e)}_e("MuiLinearProgress",["root","colorPrimary","colorSecondary","determinate","indeterminate","buffer","query","dashed","dashedColorPrimary","dashedColorSecondary","bar","bar1","bar2","barColorPrimary","barColorSecondary","bar1Indeterminate","bar1Determinate","bar1Buffer","bar2Indeterminate","bar2Buffer"]);const Oe=4,ze=Ye`
  0% {
    left: -35%;
    right: 100%;
  }

  60% {
    left: 100%;
    right: -90%;
  }

  100% {
    left: 100%;
    right: -90%;
  }
`,Br=typeof ze!="string"?qe`
        animation: ${ze} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite;
      `:null,$e=Ye`
  0% {
    left: -200%;
    right: 100%;
  }

  60% {
    left: 107%;
    right: -8%;
  }

  100% {
    left: 107%;
    right: -8%;
  }
`,Hr=typeof $e!="string"?qe`
        animation: ${$e} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite;
      `:null,Be=Ye`
  0% {
    opacity: 1;
    background-position: 0 -23px;
  }

  60% {
    opacity: 0;
    background-position: 0 -23px;
  }

  100% {
    opacity: 1;
    background-position: -200px -23px;
  }
`,Ur=typeof Be!="string"?qe`
        animation: ${Be} 3s infinite linear;
      `:null,Vr=e=>{const{classes:n,variant:o,color:r}=e,c={root:["root",`color${T(r)}`,o],dashed:["dashed",`dashedColor${T(r)}`],bar1:["bar","bar1",`barColor${T(r)}`,(o==="indeterminate"||o==="query")&&"bar1Indeterminate",o==="determinate"&&"bar1Determinate",o==="buffer"&&"bar1Buffer"],bar2:["bar","bar2",o!=="buffer"&&`barColor${T(r)}`,o==="buffer"&&`color${T(r)}`,(o==="indeterminate"||o==="query")&&"bar2Indeterminate",o==="buffer"&&"bar2Buffer"]};return We(c,$r,n)},Xe=(e,n)=>e.vars?e.vars.palette.LinearProgress[`${n}Bg`]:e.palette.mode==="light"?e.lighten(e.palette[n].main,.62):e.darken(e.palette[n].main,.5),Gr=z("span",{name:"MuiLinearProgress",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`color${T(o.color)}`],n[o.variant]]}})(W(({theme:e})=>({position:"relative",overflow:"hidden",display:"block",height:4,zIndex:0,"@media print":{colorAdjust:"exact"},variants:[...Object.entries(e.palette).filter(X()).map(([n])=>({props:{color:n},style:{backgroundColor:Xe(e,n)}})),{props:({ownerState:n})=>n.color==="inherit"&&n.variant!=="buffer",style:{"&::before":{content:'""',position:"absolute",left:0,top:0,right:0,bottom:0,backgroundColor:"currentColor",opacity:.3}}},{props:{variant:"buffer"},style:{backgroundColor:"transparent"}},{props:{variant:"query"},style:{transform:"rotate(180deg)"}}]}))),_r=z("span",{name:"MuiLinearProgress",slot:"Dashed",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.dashed,n[`dashedColor${T(o.color)}`]]}})(W(({theme:e})=>({position:"absolute",marginTop:0,height:"100%",width:"100%",backgroundSize:"10px 10px",backgroundPosition:"0 -23px",variants:[{props:{color:"inherit"},style:{opacity:.3,backgroundImage:"radial-gradient(currentColor 0%, currentColor 16%, transparent 42%)"}},...Object.entries(e.palette).filter(X()).map(([n])=>{const o=Xe(e,n);return{props:{color:n},style:{backgroundImage:`radial-gradient(${o} 0%, ${o} 16%, transparent 42%)`}}})]})),Ur||{animation:`${Be} 3s infinite linear`}),Wr=z("span",{name:"MuiLinearProgress",slot:"Bar1",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar1,n[`barColor${T(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar1Indeterminate,o.variant==="determinate"&&n.bar1Determinate,o.variant==="buffer"&&n.bar1Buffer]}})(W(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[{props:{color:"inherit"},style:{backgroundColor:"currentColor"}},...Object.entries(e.palette).filter(X()).map(([n])=>({props:{color:n},style:{backgroundColor:(e.vars||e).palette[n].main}})),{props:{variant:"determinate"},style:{transition:`transform .${Oe}s linear`}},{props:{variant:"buffer"},style:{zIndex:1,transition:`transform .${Oe}s linear`}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:Br||{animation:`${ze} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite`}}]}))),qr=z("span",{name:"MuiLinearProgress",slot:"Bar2",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar2,n[`barColor${T(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar2Indeterminate,o.variant==="buffer"&&n.bar2Buffer]}})(W(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[...Object.entries(e.palette).filter(X()).map(([n])=>({props:{color:n},style:{"--LinearProgressBar2-barColor":(e.vars||e).palette[n].main}})),{props:({ownerState:n})=>n.variant!=="buffer"&&n.color!=="inherit",style:{backgroundColor:"var(--LinearProgressBar2-barColor, currentColor)"}},{props:({ownerState:n})=>n.variant!=="buffer"&&n.color==="inherit",style:{backgroundColor:"currentColor"}},{props:{color:"inherit"},style:{opacity:.3}},...Object.entries(e.palette).filter(X()).map(([n])=>({props:{color:n,variant:"buffer"},style:{backgroundColor:Xe(e,n),transition:`transform .${Oe}s linear`}})),{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:Hr||{animation:`${$e} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite`}}]}))),Yr=p.forwardRef(function(n,o){const r=Pe({props:n,name:"MuiLinearProgress"}),{className:c,color:l="primary",value:u,valueBuffer:d,variant:m="indeterminate",...x}=r,i={...r,color:l,variant:m},g=Vr(i),A=Lr(),C={},s={bar1:{},bar2:{}};if((m==="determinate"||m==="buffer")&&u!==void 0){C["aria-valuenow"]=Math.round(u),C["aria-valuemin"]=0,C["aria-valuemax"]=100;let f=u-100;A&&(f=-f),s.bar1.transform=`translateX(${f}%)`}if(m==="buffer"&&d!==void 0){let f=(d||0)-100;A&&(f=-f),s.bar2.transform=`translateX(${f}%)`}return t.jsxs(Gr,{className:_t(g.root,c),ownerState:i,role:"progressbar",...C,ref:o,...x,children:[m==="buffer"?t.jsx(_r,{className:g.dashed,ownerState:i}):null,t.jsx(Wr,{className:g.bar1,ownerState:i,style:s.bar1}),m==="determinate"?null:t.jsx(qr,{className:g.bar2,ownerState:i,style:s.bar2})]})});function Kr(e={}){const{autoHideDuration:n=null,disableWindowBlurListener:o=!1,onClose:r,open:c,resumeHideDuration:l}=e,u=xr();p.useEffect(()=>{if(!c)return;function b(y){y.defaultPrevented||y.key==="Escape"&&(r==null||r(y,"escapeKeyDown"))}return document.addEventListener("keydown",b),()=>{document.removeEventListener("keydown",b)}},[c,r]);const d=Ne((b,y)=>{r==null||r(b,y)}),m=Ne(b=>{!r||b==null||u.start(b,()=>{d(null,"timeout")})});p.useEffect(()=>(c&&m(n),u.clear),[c,n,m,u]);const x=b=>{r==null||r(b,"clickaway")},i=u.clear,g=p.useCallback(()=>{n!=null&&m(l??n*.5)},[n,l,m]),A=b=>y=>{const j=b.onBlur;j==null||j(y),g()},C=b=>y=>{const j=b.onFocus;j==null||j(y),i()},s=b=>y=>{const j=b.onMouseEnter;j==null||j(y),i()},f=b=>y=>{const j=b.onMouseLeave;j==null||j(y),g()};return p.useEffect(()=>{if(!o&&c)return window.addEventListener("focus",g),window.addEventListener("blur",i),()=>{window.removeEventListener("focus",g),window.removeEventListener("blur",i)}},[o,c,g,i]),{getRootProps:(b={})=>{const y={...an(e),...an(b)};return{role:"presentation",...b,...y,onBlur:A(y),onFocus:C(y),onMouseEnter:s(y),onMouseLeave:f(y)}},onClickAway:x}}function Xr(e){return Ge("MuiSnackbarContent",e)}_e("MuiSnackbarContent",["root","message","action"]);const Jr=e=>{const{classes:n}=e;return We({root:["root"],action:["action"],message:["message"]},Xr,n)},Qr=z(Cr,{name:"MuiSnackbarContent",slot:"Root"})(W(({theme:e})=>{const n=e.palette.mode==="light"?.8:.98;return{...e.typography.body2,color:e.vars?e.vars.palette.SnackbarContent.color:e.palette.getContrastText(cn(e.palette.background.default,n)),backgroundColor:e.vars?e.vars.palette.SnackbarContent.bg:cn(e.palette.background.default,n),display:"flex",alignItems:"center",flexWrap:"wrap",padding:"6px 16px",flexGrow:1,[e.breakpoints.up("sm")]:{flexGrow:"initial",minWidth:288}}})),Zr=z("div",{name:"MuiSnackbarContent",slot:"Message"})({padding:"8px 0"}),es=z("div",{name:"MuiSnackbarContent",slot:"Action"})({display:"flex",alignItems:"center",marginLeft:"auto",paddingLeft:16,marginRight:-8}),ns=p.forwardRef(function(n,o){const r=Pe({props:n,name:"MuiSnackbarContent"}),{action:c,className:l,message:u,role:d="alert",...m}=r,x=r,i=Jr(x);return t.jsxs(Qr,{role:d,elevation:6,className:_t(i.root,l),ownerState:x,ref:o,...m,children:[t.jsx(Zr,{className:i.message,ownerState:x,children:u}),c?t.jsx(es,{className:i.action,ownerState:x,children:c}):null]})});function ts(e){return Ge("MuiSnackbar",e)}_e("MuiSnackbar",["root","anchorOriginTopCenter","anchorOriginBottomCenter","anchorOriginTopRight","anchorOriginBottomRight","anchorOriginTopLeft","anchorOriginBottomLeft"]);const os=e=>{const{classes:n,anchorOrigin:o}=e,r={root:["root",`anchorOrigin${T(o.vertical)}${T(o.horizontal)}`]};return We(r,ts,n)},rs=z("div",{name:"MuiSnackbar",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`anchorOrigin${T(o.anchorOrigin.vertical)}${T(o.anchorOrigin.horizontal)}`]]}})(W(({theme:e})=>({zIndex:(e.vars||e).zIndex.snackbar,position:"fixed",display:"flex",left:8,right:8,justifyContent:"center",alignItems:"center",variants:[{props:({ownerState:n})=>n.anchorOrigin.vertical==="top",style:{top:8,[e.breakpoints.up("sm")]:{top:24}}},{props:({ownerState:n})=>n.anchorOrigin.vertical!=="top",style:{bottom:8,[e.breakpoints.up("sm")]:{bottom:24}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="left",style:{justifyContent:"flex-start",[e.breakpoints.up("sm")]:{left:24,right:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="right",style:{justifyContent:"flex-end",[e.breakpoints.up("sm")]:{right:24,left:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="center",style:{[e.breakpoints.up("sm")]:{left:"50%",right:"auto",transform:"translateX(-50%)"}}}]}))),ss=p.forwardRef(function(n,o){const r=Pe({props:n,name:"MuiSnackbar"}),c=yr(),l={enter:c.transitions.duration.enteringScreen,exit:c.transitions.duration.leavingScreen},{action:u,anchorOrigin:{vertical:d,horizontal:m}={vertical:"bottom",horizontal:"left"},autoHideDuration:x=null,children:i,className:g,ClickAwayListenerProps:A,ContentProps:C,disableWindowBlurListener:s=!1,message:f,onBlur:k,onClose:b,onFocus:y,onMouseEnter:j,onMouseLeave:q,open:$,resumeHideDuration:Q,slots:h={},slotProps:a={},TransitionComponent:v,transitionDuration:L=l,TransitionProps:{onEnter:Y,onExited:B,...no}={},...to}=r,H={...r,anchorOrigin:{vertical:d,horizontal:m},autoHideDuration:x,disableWindowBlurListener:s,TransitionComponent:v,transitionDuration:L},oo=os(H),{getRootProps:ro,onClickAway:so}=Kr({...H}),[ao,Qe]=p.useState(!0),io=R=>{Qe(!0),B&&B(R)},co=(R,N)=>{Qe(!1),Y&&Y(R,N)},Z={slots:{transition:v,...h},slotProps:{content:C,clickAwayListener:A,transition:no,...a}},[lo,uo]=ee("root",{ref:o,className:[oo.root,g],elementType:rs,getSlotProps:ro,externalForwardedProps:{...Z,...to},ownerState:H}),[po,{ownerState:mo,...go}]=ee("clickAwayListener",{elementType:Mr,externalForwardedProps:Z,getSlotProps:R=>({onClickAway:(...N)=>{var Ze;const F=N[0];(Ze=R.onClickAway)==null||Ze.call(R,...N),!(F!=null&&F.defaultMuiPrevented)&&so(...N)}}),ownerState:H}),[fo,ho]=ee("content",{elementType:ns,shouldForwardComponentProp:!0,externalForwardedProps:Z,additionalProps:{message:f,action:u},ownerState:H}),[bo,xo]=ee("transition",{elementType:jr,externalForwardedProps:Z,getSlotProps:R=>({onEnter:(...N)=>{var F;(F=R.onEnter)==null||F.call(R,...N),co(...N)},onExited:(...N)=>{var F;(F=R.onExited)==null||F.call(R,...N),io(...N)}}),additionalProps:{appear:!0,in:$,timeout:L,direction:d==="top"?"down":"up"},ownerState:H});return!$&&ao?null:t.jsx(po,{...go,...h.clickAwayListener&&{ownerState:mo},children:t.jsx(lo,{...uo,children:t.jsx(bo,{...xo,children:i||t.jsx(fo,{...ho})})})})}),as=Ve(t.jsx("path",{d:"M6 2v6h.01L6 8.01 10 12l-4 4 .01.01H6V22h12v-5.99h-.01L18 16l-4-4 4-3.99-.01-.01H18V2zm10 14.5V20H8v-3.5l4-4zm-4-5-4-4V4h8v3.5z"})),is=Ve(t.jsx("path",{d:"M12 22c1.1 0 2-.9 2-2h-4c0 1.1.9 2 2 2m6-6v-5c0-3.07-1.63-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.64 5.36 6 7.92 6 11v5l-2 2v1h16v-1zm-2 1H8v-6c0-2.48 1.51-4.5 4-4.5s4 2.02 4 4.5z"})),cs=Ve([t.jsx("path",{d:"M12 5.99 19.53 19H4.47zM12 2 1 21h22z"},"0"),t.jsx("path",{d:"M13 16h-2v2h2zm0-6h-2v5h2z"},"1")]),ls=e=>e.spec.level===on.ERROR?t.jsx(Ut,{color:"error",fontSize:"small"}):e.spec.level===on.WARNING?t.jsx(cs,{color:"warning",fontSize:"small"}):t.jsx(pr,{color:"info",fontSize:"small"}),ds=e=>{try{const n=new URL(e,window.location.origin);return n.origin!==window.location.origin?null:`${n.pathname}${n.search}${n.hash}`}catch{return null}},Jt=()=>{const e=He(),n=Vt(),o=S(Co),[r,c]=p.useState(null),[l,u]=p.useState(null),d=!!r,m=!!l,x=S(yo),i=S(vo),g=S(So),A=S(jo),C=S(wo),s=p.useMemo(()=>i.filter(h=>!h.status.viewed).map(h=>h.metadata.id),[i]);p.useEffect(()=>{o&&g==="idle"&&e(ko())},[o,e,g]);const f=h=>{c(h.currentTarget)},k=()=>{c(null)},b=()=>{k(),e(Eo())},y=h=>{u(h.currentTarget),s.length>0&&e(Io(s))},j=()=>{u(null)},q=h=>{var L;const a=(L=h.status.action)==null?void 0:L.url;if(!a)return;j();const v=ds(a);if(v){n(v);return}window.location.href=a},$=x.length==0?"":x.map((h,a)=>t.jsx(ss,{open:!0,autoHideDuration:6e3,onClose:()=>e(en(a)),anchorOrigin:{vertical:"bottom",horizontal:"center"},children:t.jsx(K,{onClose:()=>e(en(a)),severity:h.type,sx:{width:"100%"},children:h.message})},h.id)),Q=i.length===0?t.jsx(ne,{disabled:!0,children:t.jsx(rn,{primary:"No notifications",primaryTypographyProps:{variant:"body2",color:"text.secondary"}})}):i.map(h=>{var a;return t.jsxs(ne,{disableRipple:!0,sx:{alignItems:"flex-start",gap:1.5,maxWidth:420,minWidth:{xs:300,sm:380},py:1.5,whiteSpace:"normal"},children:[t.jsx(ur,{sx:{minWidth:32,pt:.25},children:ls(h)}),t.jsx(rn,{primary:h.spec.title,secondary:h.spec.message,primaryTypographyProps:{variant:"subtitle2",fontWeight:h.status.viewed?500:700},secondaryTypographyProps:{variant:"body2",color:"text.secondary",sx:{mt:.5}}}),((a=h.status.action)==null?void 0:a.url)&&t.jsx(_,{size:"small",variant:"outlined",onClick:v=>{v.stopPropagation(),q(h)},sx:{flexShrink:0,mt:.25},children:"Open"})]},h.metadata.id)});return t.jsxs(P,{sx:{display:"flex",flexDirection:"column",minHeight:"100vh",bgcolor:"background.default"},children:[o&&t.jsxs(Ke,{maxWidth:"lg",sx:{display:"flex",justifyContent:"flex-end",alignItems:"center",gap:1,pt:{xs:1,sm:2}},children:[t.jsx(ar,{title:"Open notifications",children:t.jsx(kr,{id:"notifications-button",color:"inherit",size:"small",onClick:y,"aria-controls":m?"notifications-menu":void 0,"aria-haspopup":"true","aria-expanded":m?"true":void 0,"aria-label":"Open notifications",sx:{color:"text.secondary"},children:t.jsx(ir,{badgeContent:C,color:"warning",invisible:C===0,children:t.jsx(is,{fontSize:"small"})})})}),t.jsxs(ln,{id:"notifications-menu",anchorEl:l,open:m,onClose:j,MenuListProps:{"aria-labelledby":"notifications-button"},PaperProps:{sx:{mt:1,maxHeight:480,borderRadius:w.radius.panel}},children:[t.jsxs(P,{sx:{px:2,py:1.25},children:[t.jsx(I,{variant:"subtitle1",component:"p",sx:{fontWeight:700},children:"Notifications"}),g==="failed"&&t.jsx(I,{variant:"body2",color:"error",children:A??"Could not load notifications"})]}),t.jsx(cr,{}),Q]}),t.jsx(_,{id:"account-button",onClick:f,color:"inherit",size:"small",endIcon:t.jsx(lr,{alt:o,src:"/assets/avatar.png",sx:{width:28,height:28,fontSize:14}}),"aria-controls":d?"account-menu":void 0,"aria-haspopup":"true","aria-expanded":d?"true":void 0,sx:{color:"text.secondary",minWidth:0,textTransform:"none"},children:t.jsx(I,{variant:"body2",component:"span",noWrap:!0,sx:{display:{xs:"none",sm:"inline"},maxWidth:260},children:o})}),t.jsxs(ln,{id:"account-menu",anchorEl:r,open:d,onClose:k,MenuListProps:{"aria-labelledby":"account-button"},children:[t.jsx(ne,{disabled:!0,children:t.jsx(I,{variant:"body2",children:o})}),t.jsx(ne,{onClick:b,children:"Logout"})]})]}),t.jsxs(P,{component:"main",sx:{flexGrow:1},children:[t.jsx(fr,{}),$]})]})};Jt.__docgenInfo={description:"Layout component for the application",methods:[],displayName:"Layout"};const Qt=()=>{const e=He(),n=Vt(),o=S(zt),r=S($t),c=S(Bt),{cancelForm:l,connect:u,currentFormStep:d,formSubmitError:m,isConnecting:x,isSubmittingForm:i,submitForm:g}=Ir();p.useEffect(()=>{r==="idle"&&e(Ht())},[r,e]);const A=p.useCallback(s=>{n(`/connectors/${encodeURIComponent(s)}`)},[n]);let C;return r==="loading"?C=t.jsx(M,{container:!0,spacing:w.spacing.gridGap,children:[1,2,3,4].map(s=>t.jsx(M,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Kt,{})},s))}):r==="failed"?C=t.jsx(K,{severity:"error",children:c}):o.length===0?C=t.jsx(P,{sx:{textAlign:"center",py:w.spacing.pageY},children:t.jsx(I,{variant:"h6",color:"text.secondary",children:"No connectors available"})}):C=t.jsx(M,{container:!0,spacing:w.spacing.gridGap,children:o.map(s=>t.jsx(M,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Yt,{connector:s,onConnect:u,onDetails:A,isConnecting:x})},`${s.metadata.id}:${s.metadata.generation}`))}),t.jsxs(Ke,{sx:{py:w.spacing.pageY},children:[t.jsxs(P,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:w.spacing.headerGap,mb:w.spacing.sectionGap},children:[t.jsx(I,{variant:"h4",component:"h1",children:"Available Connectors"}),t.jsxs(P,{sx:{display:"flex",alignItems:"center",gap:w.spacing.headerGap},children:[x&&t.jsxs(P,{sx:{display:"flex",alignItems:"center"},children:[t.jsx(De,{size:24,sx:{mr:1}}),t.jsx(I,{variant:"body2",color:"text.secondary",children:"Connecting..."})]}),t.jsx(_,{component:Wt,to:"/connections",startIcon:t.jsx(Er,{}),sx:{alignSelf:{xs:"flex-start",sm:"center"}},children:"Back to Connections"})]})]}),C,t.jsx(qt,{currentFormStep:d,formSubmitError:m,isSubmittingForm:i,onCancel:l,onSubmit:g})]})};Qt.__docgenInfo={description:"Component to display a list of available connectors",methods:[],displayName:"ConnectorList"};const Zt=()=>{const e=He(),[n,o]=Ar(),r=S(Ao),c=S(Po),l=S(Ro),u=S(zt),d=S($t),m=S(Bt),x=S(Lo),i=S(To),g=S(Mo),A=S(No),C=S(Do),s=S(Fo),f=S(Oo),k=S(zo);p.useEffect(()=>{c==="idle"&&e(U()),d==="idle"&&e(Ht())},[c,d,e]),p.useEffect(()=>{const a=n.get("setup"),v=n.get("connectionId");a==="pending"&&v&&(e(nn(v)),n.delete("setup"),n.delete("connectionId"),o(n,{replace:!0}))},[n,o,e]),p.useEffect(()=>{if(!C)return;const a=window.setInterval(()=>{e(nn(C))},2e3);return()=>window.clearInterval(a)},[C,e]),p.useEffect(()=>{if(!k)return;e(U());const a=window.setTimeout(()=>{e($o())},3500);return()=>window.clearTimeout(a)},[e,k]);const b=p.useCallback((a,v)=>{const L=(i==null?void 0:i.stepId)??"";e(Bo({connectionId:a,stepId:L,data:v,returnToUrl:window.location.href})).then(Y=>{if(Y.meta.requestStatus==="fulfilled"){const B=Y.payload;Le(B)?window.location.href=B.status.redirectUrl:tn(B)?e(U()):e(U())}})},[e,i]),y=p.useCallback(()=>{const a=i==null?void 0:i.connectionId,v=a?r.find(L=>L.metadata.id===a):void 0;v&&v.status.lifecycle.state===Me.CONFIGURED&&e(Ho(v.metadata.id)),e(Uo())},[e,i,r]),j=p.useCallback(()=>{s&&e(Vo({connectionId:s.connectionId,returnToUrl:window.location.href})).then(a=>{if(a.meta.requestStatus==="fulfilled"){const v=a.payload;Le(v)&&(window.location.href=v.status.redirectUrl)}})},[e,s]),q=p.useCallback(()=>{s&&e(Go(s.connectionId)).then(()=>{e(_o()),e(U())})},[e,s]),$=p.useCallback(a=>{e(Wo({connectorRef:tr(a),returnToUrl:`${window.location.origin}/connections`})).then(v=>{if(v.meta.requestStatus==="fulfilled"){const L=v.payload;Le(L)?window.location.href=L.status.redirectUrl:tn(L)&&e(U())}})},[e]),Q=()=>d==="loading"||d==="idle"?t.jsx(M,{container:!0,spacing:w.spacing.gridGap,children:[1,2,3,4].map(a=>t.jsx(M,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Kt,{})},`connector-skeleton-${a}`))}):d==="failed"?t.jsx(K,{severity:"error",children:m}):u.length===0?t.jsx(P,{sx:{py:3},children:t.jsx(I,{color:"text.secondary",children:"No connectors are available right now."})}):t.jsx(M,{container:!0,spacing:w.spacing.gridGap,children:u.map(a=>t.jsx(M,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Yt,{connector:a,onConnect:$,isConnecting:x})},`${a.metadata.id}:${a.metadata.generation}`))});let h;return c==="loading"?h=t.jsx(M,{container:!0,spacing:w.spacing.gridGap,children:[1,2,3,4].map(a=>t.jsx(M,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(gr,{})},`connection-skeleton-${a}`))}):c==="failed"?h=t.jsx(K,{severity:"error",children:l}):r.length===0?h=t.jsxs(t.Fragment,{children:[t.jsxs(P,{sx:{border:1,borderColor:w.card.borderColor,borderRadius:w.radius.panel,bgcolor:w.card.surface,mb:w.spacing.sectionGap,p:w.spacing.panelPadding},children:[t.jsx(I,{variant:"h5",component:"h2",gutterBottom:!0,children:"Connect your first application"}),t.jsx(I,{color:"text.secondary",sx:{maxWidth:680},children:"Choose a connector below to create a connection. Once connected, it will appear here for ongoing setup, health, and management."}),x&&t.jsxs(P,{sx:{display:"flex",alignItems:"center",mt:3},children:[t.jsx(De,{size:24,sx:{mr:1}}),t.jsx(I,{variant:"body2",color:"text.secondary",children:"Starting connection..."})]})]}),t.jsxs(P,{children:[t.jsx(I,{variant:"h6",component:"h2",sx:{mb:2},children:"Available connectors"}),Q()]})]}):h=t.jsx(M,{container:!0,spacing:w.spacing.gridGap,children:r.map(a=>t.jsx(M,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(mr,{connection:a,connector:u.find(v=>or(v,a.spec.connectorRef)),highlightNew:a.metadata.id===k})},a.metadata.id))}),t.jsxs(Ke,{sx:{py:w.spacing.pageY},children:[t.jsxs(P,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:w.spacing.headerGap,mb:w.spacing.sectionGap},children:[t.jsx(I,{variant:"h4",component:"h1",children:"Your Connections"}),r.length>0&&t.jsx(_,{variant:"contained",color:"primary",startIcon:t.jsx(dr,{}),component:Wt,to:"/connectors",children:"Connect More"})]}),h,t.jsx(qt,{currentFormStep:i,formSubmitError:A,isSubmittingForm:g,onCancel:y,onSubmit:b}),t.jsxs(dn,{open:C!==null,maxWidth:"xs",fullWidth:!0,children:[t.jsx(un,{sx:{pb:1},children:"Verifying connection"}),t.jsx(pn,{dividers:!0,children:t.jsxs(P,{sx:{display:"flex",flexDirection:"column",alignItems:"center",gap:w.spacing.headerGap,py:3},children:[t.jsx(as,{color:"primary",sx:{fontSize:40}}),t.jsxs(P,{sx:{textAlign:"center"},children:[t.jsx(I,{variant:"subtitle1",component:"p",children:"Checking credentials"}),t.jsx(I,{variant:"body2",color:"text.secondary",children:"AuthProxy is confirming that this connection can reach the provider."})]}),t.jsx(Yr,{sx:{width:"100%"}})]})})]}),t.jsxs(dn,{open:s!==null,onClose:q,maxWidth:"sm",fullWidth:!0,children:[t.jsx(un,{sx:{pb:1},children:t.jsxs(P,{sx:{display:"flex",alignItems:"center",gap:1},children:[t.jsx(Ut,{color:"error"}),t.jsx(I,{variant:"h6",component:"span",children:"Connection verification failed"})]})}),t.jsxs(pn,{dividers:!0,children:[t.jsxs(K,{severity:"error",sx:{mb:2},children:[t.jsx(Pr,{children:"Provider check failed"}),(s==null?void 0:s.message)??"Verification failed"]}),t.jsx(I,{variant:"body2",color:"text.secondary",children:s!=null&&s.canRetry?"Retry setup to run verification again. Cancel setup deletes this unfinished connection.":"Cancel setup to delete this unfinished connection, then start again from the connector."})]}),t.jsxs(wr,{children:[t.jsx(_,{onClick:q,disabled:f,children:"Cancel setup"}),(s==null?void 0:s.canRetry)&&t.jsx(_,{onClick:j,disabled:f,variant:"contained",startIcon:f?t.jsx(De,{size:16}):void 0,children:f?"Retrying setup...":"Retry setup"})]})]})]})};Zt.__docgenInfo={description:"Component to display a list of connections",methods:[],displayName:"ConnectionList"};const G=(e,n,o="#ffffff")=>{const r=e.split(/\s+/).map(l=>l[0]).join("").slice(0,2).toUpperCase(),c=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${n}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="${o}" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">${r}</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(c)}`},D=[V({id:"google-drive",displayName:"Google Drive",description:"Have the agent track your work in Google Drive.",highlight:"Have the agent track your work in Google Drive.",logo:{publicUrl:G("Google Drive","#188038")},hasConfigure:!1}),V({id:"greenhouse",displayName:"Greenhouse",description:"This integration pushes candidates to greenhouse.",highlight:"This integration pushes candidates to greenhouse.",logo:{publicUrl:G("Greenhouse","#24a47f")},hasConfigure:!1}),V({id:"google-calendar",displayName:"Google Calendar",description:`Google Calendar lets agents coordinate scheduling work without needing direct access to your primary app.

![Calendar workflow preview](/calendar-workflow-preview.svg)

### What agents can do

| Capability | Supported |
| --- | --- |
| Find open time | Yes |
| Create and update events | Yes |
| Read attendee responses | Yes |
| Manage private event details | No |

Use this connector when the assistant should propose meeting times, create holds, or keep follow-up work attached to calendar events.`,highlight:"Coordinate meetings, availability, and follow-up from Google Calendar.",logo:{publicUrl:G("Google Calendar","#1a73e8")},hasConfigure:!0}),V({id:"gmail",displayName:"GMail",description:"Have the agent respond to your emails without you needing to be involved. Like magic.",highlight:"Have the agent respond to your emails without you needing to be involved. Like magic.",logo:{publicUrl:G("GMail","#d93025")},hasConfigure:!1}),V({id:"pipedrive",displayName:"pipedrive",description:"Allow our agent to handle your sales support.",highlight:"Allow our agent to handle your sales support.",logo:{publicUrl:G("pipedrive","#017a5e")},hasConfigure:!1}),V({id:"asana",displayName:"Asana",description:"Allow our agent organize your work.",highlight:"Allow our agent organize your work.",logo:{publicUrl:G("Asana","#f06a6a")},hasConfigure:!1})],O=(e,n={})=>rr({id:`cxn_${e.metadata.id}`,connector:e,...n}),us=[O(D[0]),O(D[2],{healthState:Ue.UNHEALTHY}),O(D[5],{state:Me.SETUP}),O(D[4],{state:Me.DISABLED})],Je={connectionId:"cxn_google-calendar",stepId:"select-calendar",stepTitle:"Select a Calendar",stepDescription:"Choose which Google Calendar the agent should manage.",currentStep:0,totalSteps:2,jsonSchema:{type:"object",required:["calendar_id"],properties:{calendar_id:{type:"string",title:"Calendar",enum:["primary","product","support"]}}},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/calendar_id"}]}},E={items:us,status:"succeeded",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null,currentFormStep:null,submittingForm:!1,formSubmitError:null,verifyingConnectionId:null,verifyError:null,retryingConnection:!1,recentlyCompletedConnectionId:null},eo={items:[],status:"succeeded",error:null,markingViewed:!1};function ps({route:e,colorMode:n="light",connectorsState:o={items:D,status:"succeeded",error:null},connectionsState:r=E,notificationsState:c=eo}){const l=p.useMemo(()=>nr(n),[n]),u=qo({reducer:Yo({auth:er,connectors:Zo,connections:Qo,notifications:Jo,toasts:Xo}),preloadedState:{auth:{actorId:"actor_storybook",status:"authenticated"},connectors:o,connections:r,notifications:c,toasts:{items:[]}}});return t.jsx(Ko,{store:u,children:t.jsxs(vr,{theme:l,children:[t.jsx(zr,{enableColorScheme:!0}),t.jsx(hr,{children:t.jsx(sn,{element:t.jsx(Jt,{}),children:t.jsx(sn,{path:"*",element:e==="/connectors"?t.jsx(Qt,{}):e==="/connector-detail"?t.jsx(Rr,{connectorId:"google-calendar"}):t.jsx(Zt,{})})})})]})})}const zs={title:"Pages/Marketplace",component:ps,parameters:{layout:"fullscreen"}},Re={viewport:{defaultViewport:"marketplaceMobile"}},J={viewport:{defaultViewport:"marketplaceTablet"}},te={args:{route:"/connectors"}},oe={args:{route:"/connectors",colorMode:"dark"}},re={args:{route:"/connector-detail"}},se={args:{route:"/connector-detail"},parameters:Re},ae={args:{route:"/connectors",connectorsState:{items:[],status:"loading",error:null}}},ie={args:{route:"/connections"}},ce={args:{route:"/connections",colorMode:"dark"}},le={args:{route:"/connections"},parameters:Re},de={args:{route:"/connections"},parameters:J},ue={args:{route:"/connections",connectionsState:{...E,items:[O(D[2],{healthState:Ue.UNHEALTHY})]}}},pe={args:{route:"/connections",connectionsState:{...E,items:[O(D[2],{healthState:Ue.UNHEALTHY}),O(D[5],{setupStepId:"select-workspace"})]},notificationsState:{...eo,items:[sr({id:"ntf_reauth",actionUrl:"/connections/cxn_google-calendar?action=reauth"})]}}},me={args:{route:"/connections",connectionsState:{...E,items:[O(D[2]),O(D[0])]}}},ge={args:{route:"/connections",connectionsState:{...E,items:[]}}},fe={args:{route:"/connections",connectionsState:{...E,items:[]}},parameters:Re},he={args:{route:"/connections",connectionsState:{...E,items:[]}},parameters:J},be={args:{route:"/connections",connectorsState:{items:[],status:"loading",error:null},connectionsState:{...E,items:[]}}},xe={args:{route:"/connectors"},parameters:Re},Ce={args:{route:"/connectors"},parameters:J},ye={args:{route:"/connections",connectionsState:{...E,currentFormStep:Je}}},ve={args:{route:"/connections",connectionsState:{...E,currentFormStep:Je}},parameters:J},Se={args:{route:"/connections",connectionsState:{...E,currentFormStep:Je,submittingForm:!0}}},je={args:{route:"/connections",connectionsState:{...E,verifyingConnectionId:"cxn_google-calendar"}}},we={args:{route:"/connections",connectionsState:{...E,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}}},ke={args:{route:"/connections",connectionsState:{...E,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}},parameters:J},Ie={args:{route:"/connections",connectionsState:{...E,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0},retryingConnection:!0}}},Ee={args:{route:"/connections",connectionsState:{...E,verifyError:{connectionId:"cxn_google-calendar",message:"The provider rejected this setup and it cannot be retried.",canRetry:!1}}}};var gn,fn,hn;te.parameters={...te.parameters,docs:{...(gn=te.parameters)==null?void 0:gn.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  }
}`,...(hn=(fn=te.parameters)==null?void 0:fn.docs)==null?void 0:hn.source}}};var bn,xn,Cn;oe.parameters={...oe.parameters,docs:{...(bn=oe.parameters)==null?void 0:bn.docs,source:{originalSource:`{
  args: {
    route: '/connectors',
    colorMode: 'dark'
  }
}`,...(Cn=(xn=oe.parameters)==null?void 0:xn.docs)==null?void 0:Cn.source}}};var yn,vn,Sn;re.parameters={...re.parameters,docs:{...(yn=re.parameters)==null?void 0:yn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  }
}`,...(Sn=(vn=re.parameters)==null?void 0:vn.docs)==null?void 0:Sn.source}}};var jn,wn,kn;se.parameters={...se.parameters,docs:{...(jn=se.parameters)==null?void 0:jn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  },
  parameters: mobileViewport
}`,...(kn=(wn=se.parameters)==null?void 0:wn.docs)==null?void 0:kn.source}}};var In,En,An;ae.parameters={...ae.parameters,docs:{...(In=ae.parameters)==null?void 0:In.docs,source:{originalSource:`{
  args: {
    route: '/connectors',
    connectorsState: {
      items: [],
      status: 'loading',
      error: null
    }
  }
}`,...(An=(En=ae.parameters)==null?void 0:En.docs)==null?void 0:An.source}}};var Pn,Rn,Ln;ie.parameters={...ie.parameters,docs:{...(Pn=ie.parameters)==null?void 0:Pn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  }
}`,...(Ln=(Rn=ie.parameters)==null?void 0:Rn.docs)==null?void 0:Ln.source}}};var Tn,Mn,Nn;ce.parameters={...ce.parameters,docs:{...(Tn=ce.parameters)==null?void 0:Tn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    colorMode: 'dark'
  }
}`,...(Nn=(Mn=ce.parameters)==null?void 0:Mn.docs)==null?void 0:Nn.source}}};var Dn,Fn,On;le.parameters={...le.parameters,docs:{...(Dn=le.parameters)==null?void 0:Dn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: mobileViewport
}`,...(On=(Fn=le.parameters)==null?void 0:Fn.docs)==null?void 0:On.source}}};var zn,$n,Bn;de.parameters={...de.parameters,docs:{...(zn=de.parameters)==null?void 0:zn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: tabletViewport
}`,...(Bn=($n=de.parameters)==null?void 0:$n.docs)==null?void 0:Bn.source}}};var Hn,Un,Vn;ue.parameters={...ue.parameters,docs:{...(Hn=ue.parameters)==null?void 0:Hn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2], {
        healthState: ConnectionHealthState.UNHEALTHY
      })]
    }
  }
}`,...(Vn=(Un=ue.parameters)==null?void 0:Un.docs)==null?void 0:Vn.source}}};var Gn,_n,Wn;pe.parameters={...pe.parameters,docs:{...(Gn=pe.parameters)==null?void 0:Gn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2], {
        healthState: ConnectionHealthState.UNHEALTHY
      }), connectionFor(connectors[5], {
        setupStepId: 'select-workspace'
      })]
    },
    notificationsState: {
      ...baseNotificationsState,
      items: [notificationFixture({
        id: 'ntf_reauth',
        actionUrl: '/connections/cxn_google-calendar?action=reauth'
      })]
    }
  }
}`,...(Wn=(_n=pe.parameters)==null?void 0:_n.docs)==null?void 0:Wn.source}}};var qn,Yn,Kn;me.parameters={...me.parameters,docs:{...(qn=me.parameters)==null?void 0:qn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2]), connectionFor(connectors[0])]
    }
  }
}`,...(Kn=(Yn=me.parameters)==null?void 0:Yn.docs)==null?void 0:Kn.source}}};var Xn,Jn,Qn;ge.parameters={...ge.parameters,docs:{...(Xn=ge.parameters)==null?void 0:Xn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  }
}`,...(Qn=(Jn=ge.parameters)==null?void 0:Jn.docs)==null?void 0:Qn.source}}};var Zn,et,nt;fe.parameters={...fe.parameters,docs:{...(Zn=fe.parameters)==null?void 0:Zn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: mobileViewport
}`,...(nt=(et=fe.parameters)==null?void 0:et.docs)==null?void 0:nt.source}}};var tt,ot,rt;he.parameters={...he.parameters,docs:{...(tt=he.parameters)==null?void 0:tt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: tabletViewport
}`,...(rt=(ot=he.parameters)==null?void 0:ot.docs)==null?void 0:rt.source}}};var st,at,it;be.parameters={...be.parameters,docs:{...(st=be.parameters)==null?void 0:st.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectorsState: {
      items: [],
      status: 'loading',
      error: null
    },
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  }
}`,...(it=(at=be.parameters)==null?void 0:at.docs)==null?void 0:it.source}}};var ct,lt,dt;xe.parameters={...xe.parameters,docs:{...(ct=xe.parameters)==null?void 0:ct.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: mobileViewport
}`,...(dt=(lt=xe.parameters)==null?void 0:lt.docs)==null?void 0:dt.source}}};var ut,pt,mt;Ce.parameters={...Ce.parameters,docs:{...(ut=Ce.parameters)==null?void 0:ut.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: tabletViewport
}`,...(mt=(pt=Ce.parameters)==null?void 0:pt.docs)==null?void 0:mt.source}}};var gt,ft,ht;ye.parameters={...ye.parameters,docs:{...(gt=ye.parameters)==null?void 0:gt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  }
}`,...(ht=(ft=ye.parameters)==null?void 0:ft.docs)==null?void 0:ht.source}}};var bt,xt,Ct;ve.parameters={...ve.parameters,docs:{...(bt=ve.parameters)==null?void 0:bt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  },
  parameters: tabletViewport
}`,...(Ct=(xt=ve.parameters)==null?void 0:xt.docs)==null?void 0:Ct.source}}};var yt,vt,St;Se.parameters={...Se.parameters,docs:{...(yt=Se.parameters)==null?void 0:yt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep,
      submittingForm: true
    }
  }
}`,...(St=(vt=Se.parameters)==null?void 0:vt.docs)==null?void 0:St.source}}};var jt,wt,kt;je.parameters={...je.parameters,docs:{...(jt=je.parameters)==null?void 0:jt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyingConnectionId: 'cxn_google-calendar'
    }
  }
}`,...(kt=(wt=je.parameters)==null?void 0:wt.docs)==null?void 0:kt.source}}};var It,Et,At;we.parameters={...we.parameters,docs:{...(It=we.parameters)==null?void 0:It.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true
      }
    }
  }
}`,...(At=(Et=we.parameters)==null?void 0:Et.docs)==null?void 0:At.source}}};var Pt,Rt,Lt;ke.parameters={...ke.parameters,docs:{...(Pt=ke.parameters)==null?void 0:Pt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true
      }
    }
  },
  parameters: tabletViewport
}`,...(Lt=(Rt=ke.parameters)==null?void 0:Rt.docs)==null?void 0:Lt.source}}};var Tt,Mt,Nt;Ie.parameters={...Ie.parameters,docs:{...(Tt=Ie.parameters)==null?void 0:Tt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'Calendar API rejected the saved credentials.',
        canRetry: true
      },
      retryingConnection: true
    }
  }
}`,...(Nt=(Mt=Ie.parameters)==null?void 0:Mt.docs)==null?void 0:Nt.source}}};var Dt,Ft,Ot;Ee.parameters={...Ee.parameters,docs:{...(Dt=Ee.parameters)==null?void 0:Dt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyError: {
        connectionId: 'cxn_google-calendar',
        message: 'The provider rejected this setup and it cannot be retried.',
        canRetry: false
      }
    }
  }
}`,...(Ot=(Ft=Ee.parameters)==null?void 0:Ft.docs)==null?void 0:Ot.source}}};const $s=["AvailableConnectors","AvailableConnectorsDark","ConnectorOverview","ConnectorOverviewMobile","AvailableConnectorsLoading","ConnectionsPopulated","ConnectionsPopulatedDark","ConnectionsPopulatedMobile","ConnectionsPopulatedTablet","ConnectionsNeedsAttention","ConnectionsWithNotifications","ConnectionsHealthyActions","ConnectionsEmpty","ConnectionsEmptyMobile","ConnectionsEmptyTablet","ConnectionsEmptyLoadingConnectors","AvailableConnectorsMobile","AvailableConnectorsTablet","ConnectionSetupDialog","ConnectionSetupDialogTablet","ConnectionSetupSubmitting","VerifyingConnectionDialog","VerificationFailedDialog","VerificationFailedDialogTablet","VerificationRetryingDialog","VerificationFailedNoRetryDialog"];export{te as AvailableConnectors,oe as AvailableConnectorsDark,ae as AvailableConnectorsLoading,xe as AvailableConnectorsMobile,Ce as AvailableConnectorsTablet,ye as ConnectionSetupDialog,ve as ConnectionSetupDialogTablet,Se as ConnectionSetupSubmitting,ge as ConnectionsEmpty,be as ConnectionsEmptyLoadingConnectors,fe as ConnectionsEmptyMobile,he as ConnectionsEmptyTablet,me as ConnectionsHealthyActions,ue as ConnectionsNeedsAttention,ie as ConnectionsPopulated,ce as ConnectionsPopulatedDark,le as ConnectionsPopulatedMobile,de as ConnectionsPopulatedTablet,pe as ConnectionsWithNotifications,re as ConnectorOverview,se as ConnectorOverviewMobile,we as VerificationFailedDialog,ke as VerificationFailedDialogTablet,Ee as VerificationFailedNoRetryDialog,Ie as VerificationRetryingDialog,je as VerifyingConnectionDialog,$s as __namedExportsOrder,zs as default};

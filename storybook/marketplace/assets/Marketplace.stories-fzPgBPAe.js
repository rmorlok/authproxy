import{j as t}from"./jsx-runtime-BjG_zV1W.js";import{u as Be,e as S,s as io,f as co,g as en,t as lo,h as xt,i as vt,j as St,k as jt,l as wt,m as kt,n as Et,o as Tt,p as At,q as Rt,r as Pt,v as It,w as uo,x as po,y as mo,z as go,A as fo,B as ho,D as Te,E as nn,F as bo,G as yo,H as Co,C as xo,a as vo,c as So,I as jo,P as wo,J as ko,b as Eo,d as To,K as Ao}from"./ConnectionCard-DZmlsVdF.js";import"./client-0PMYh2xf.js";import{C as I}from"./index-BcFHX6i3.js";import{g as Ro,o as Ae,G as Po,A as Io,M as Lo,b as tn,D as Re,c as Pe,d as Ie,i as Se,a as je,e as _o,C as Ue}from"./connections-DEdR_kbc.js";import{m as j,t as Mo}from"./theme-D4YlWSlu.js";import{r as m}from"./index-yIsmwZOr.js";import{b as Do,d as Le,e as Oo,P as Vo,B as Z}from"./Button-DiNVVHLo.js";import{e as on,u as Q,c as Lt}from"./useSlot-CvJKuwFf.js";import{T as w,g as _t,u as zo}from"./Typography-zR6YQtWt.js";import{i as we,l as ke,u as Y,s as O,a as Ge,g as Ee,o as B,n as P,p as K,A as He,B as Ne,K as rn,L as $o}from"./createSimplePaletteValueFilter-DRANIdZ4.js";import{A as N,C as We}from"./Container-_1bBWRH5.js";import{B as T,C as _e}from"./Box-qKFn2-pG.js";import{N as Fo,s as Bo,a as Uo,b as Mt,u as Dt,c as Go,d as sn,O as Ho,R as No,e as an}from"./index-DbQVtLel.js";import{C as Ot,a as Vt}from"./ConnectorCard-DRUOxTFE.js";import{C as Wo,G as L,A as qo,E as Zo}from"./ConnectionFormStep-BFL-b66m.js";import"./index-M3uX8AIl.js";import{u as Ko}from"./Chip-NDjmWq1o.js";import"./IconButton-VjDeSjWN.js";import"./Close-CvQ38W9a.js";import"./useThemeProps-oZcovqdW.js";import"./Stack-Cf6NjqW7.js";function Yo(e){return we("MuiAlertTitle",e)}ke("MuiAlertTitle",["root"]);const Xo=e=>{const{classes:n}=e;return Ee({root:["root"]},Yo,n)},Jo=O(w,{name:"MuiAlertTitle",slot:"Root"})(B(({theme:e})=>({fontWeight:e.typography.fontWeightMedium,marginTop:-2}))),zt=m.forwardRef(function(n,o){const r=Y({props:n,name:"MuiAlertTitle"}),{className:s,...a}=r,d=r,l=Xo(d);return t.jsx(Jo,{gutterBottom:!0,component:"div",ownerState:d,ref:o,className:Ge(l.root,s),...a})});function cn(e){return e.substring(2).toLowerCase()}function Qo(e,n){return n.documentElement.clientWidth<e.clientX||n.documentElement.clientHeight<e.clientY}function er(e){const{children:n,disableReactTree:o=!1,mouseEvent:r="onClick",onClickAway:s,touchEvent:a="onTouchEnd"}=e,d=m.useRef(!1),l=m.useRef(null),f=m.useRef(!1),u=m.useRef(!1);m.useEffect(()=>(setTimeout(()=>{f.current=!0},0),()=>{f.current=!1}),[]);const c=Do(Ro(n),l),p=Le(i=>{const g=u.current;u.current=!1;const k=Ae(l.current);if(!f.current||!l.current||"clientX"in i&&Qo(i,k))return;if(d.current){d.current=!1;return}let h;i.composedPath?h=i.composedPath().includes(l.current):h=!k.documentElement.contains(i.target)||l.current.contains(i.target),!h&&(o||!g)&&s(i)}),C=i=>g=>{u.current=!0;const k=n.props[i];k&&k(g)},v={ref:c};return a!==!1&&(v[a]=C(a)),m.useEffect(()=>{if(a!==!1){const i=cn(a),g=Ae(l.current),k=()=>{d.current=!0};return g.addEventListener(i,p),g.addEventListener("touchmove",k),()=>{g.removeEventListener(i,p),g.removeEventListener("touchmove",k)}}},[p,a]),r!==!1&&(v[r]=C(r)),m.useEffect(()=>{if(r!==!1){const i=cn(r),g=Ae(l.current);return g.addEventListener(i,p),()=>{g.removeEventListener(i,p)}}},[p,r]),m.cloneElement(n,v)}const Me=typeof _t({})=="function",nr=(e,n)=>({WebkitFontSmoothing:"antialiased",MozOsxFontSmoothing:"grayscale",boxSizing:"border-box",WebkitTextSizeAdjust:"100%",...n&&!e.vars&&{colorScheme:e.palette.mode}}),tr=e=>({color:(e.vars||e).palette.text.primary,...e.typography.body1,backgroundColor:(e.vars||e).palette.background.default,"@media print":{backgroundColor:(e.vars||e).palette.common.white}}),$t=(e,n=!1)=>{var a,d;const o={};n&&e.colorSchemes&&typeof e.getColorSchemeSelector=="function"&&Object.entries(e.colorSchemes).forEach(([l,f])=>{var c,p;const u=e.getColorSchemeSelector(l);u.startsWith("@")?o[u]={":root":{colorScheme:(c=f.palette)==null?void 0:c.mode}}:o[u.replace(/\s*&/,"")]={colorScheme:(p=f.palette)==null?void 0:p.mode}});let r={html:nr(e,n),"*, *::before, *::after":{boxSizing:"inherit"},"strong, b":{fontWeight:e.typography.fontWeightBold},body:{margin:0,...tr(e),"&::backdrop":{backgroundColor:(e.vars||e).palette.background.default}},...o};const s=(d=(a=e.components)==null?void 0:a.MuiCssBaseline)==null?void 0:d.styleOverrides;return s&&(r=[r,s]),r},ve="mui-ecs",or=e=>{const n=$t(e,!1),o=Array.isArray(n)?n[0]:n;return!e.vars&&o&&(o.html[`:root:has(${ve})`]={colorScheme:e.palette.mode}),e.colorSchemes&&Object.entries(e.colorSchemes).forEach(([r,s])=>{var d,l;const a=e.getColorSchemeSelector(r);a.startsWith("@")?o[a]={[`:root:not(:has(.${ve}))`]:{colorScheme:(d=s.palette)==null?void 0:d.mode}}:o[a.replace(/\s*&/,"")]={[`&:not(:has(.${ve}))`]:{colorScheme:(l=s.palette)==null?void 0:l.mode}}}),n},rr=_t(Me?({theme:e,enableColorScheme:n})=>$t(e,n):({theme:e})=>or(e));function sr(e){const n=Y({props:e,name:"MuiCssBaseline"}),{children:o,enableColorScheme:r=!1}=n;return t.jsxs(m.Fragment,{children:[Me&&t.jsx(rr,{enableColorScheme:r}),!Me&&!r&&t.jsx("span",{className:ve,style:{display:"none"}}),o]})}function ar(e){return we("MuiLinearProgress",e)}ke("MuiLinearProgress",["root","colorPrimary","colorSecondary","determinate","indeterminate","buffer","query","dashed","dashedColorPrimary","dashedColorSecondary","bar","bar1","bar2","barColorPrimary","barColorSecondary","bar1Indeterminate","bar1Determinate","bar1Buffer","bar2Indeterminate","bar2Buffer"]);const De=4,Oe=Ne`
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
`,ir=typeof Oe!="string"?He`
        animation: ${Oe} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite;
      `:null,Ve=Ne`
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
`,cr=typeof Ve!="string"?He`
        animation: ${Ve} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite;
      `:null,ze=Ne`
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
`,lr=typeof ze!="string"?He`
        animation: ${ze} 3s infinite linear;
      `:null,dr=e=>{const{classes:n,variant:o,color:r}=e,s={root:["root",`color${P(r)}`,o],dashed:["dashed",`dashedColor${P(r)}`],bar1:["bar","bar1",`barColor${P(r)}`,(o==="indeterminate"||o==="query")&&"bar1Indeterminate",o==="determinate"&&"bar1Determinate",o==="buffer"&&"bar1Buffer"],bar2:["bar","bar2",o!=="buffer"&&`barColor${P(r)}`,o==="buffer"&&`color${P(r)}`,(o==="indeterminate"||o==="query")&&"bar2Indeterminate",o==="buffer"&&"bar2Buffer"]};return Ee(s,ar,n)},qe=(e,n)=>e.vars?e.vars.palette.LinearProgress[`${n}Bg`]:e.palette.mode==="light"?e.lighten(e.palette[n].main,.62):e.darken(e.palette[n].main,.5),ur=O("span",{name:"MuiLinearProgress",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`color${P(o.color)}`],n[o.variant]]}})(B(({theme:e})=>({position:"relative",overflow:"hidden",display:"block",height:4,zIndex:0,"@media print":{colorAdjust:"exact"},variants:[...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{backgroundColor:qe(e,n)}})),{props:({ownerState:n})=>n.color==="inherit"&&n.variant!=="buffer",style:{"&::before":{content:'""',position:"absolute",left:0,top:0,right:0,bottom:0,backgroundColor:"currentColor",opacity:.3}}},{props:{variant:"buffer"},style:{backgroundColor:"transparent"}},{props:{variant:"query"},style:{transform:"rotate(180deg)"}}]}))),pr=O("span",{name:"MuiLinearProgress",slot:"Dashed",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.dashed,n[`dashedColor${P(o.color)}`]]}})(B(({theme:e})=>({position:"absolute",marginTop:0,height:"100%",width:"100%",backgroundSize:"10px 10px",backgroundPosition:"0 -23px",variants:[{props:{color:"inherit"},style:{opacity:.3,backgroundImage:"radial-gradient(currentColor 0%, currentColor 16%, transparent 42%)"}},...Object.entries(e.palette).filter(K()).map(([n])=>{const o=qe(e,n);return{props:{color:n},style:{backgroundImage:`radial-gradient(${o} 0%, ${o} 16%, transparent 42%)`}}})]})),lr||{animation:`${ze} 3s infinite linear`}),mr=O("span",{name:"MuiLinearProgress",slot:"Bar1",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar1,n[`barColor${P(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar1Indeterminate,o.variant==="determinate"&&n.bar1Determinate,o.variant==="buffer"&&n.bar1Buffer]}})(B(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[{props:{color:"inherit"},style:{backgroundColor:"currentColor"}},...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{backgroundColor:(e.vars||e).palette[n].main}})),{props:{variant:"determinate"},style:{transition:`transform .${De}s linear`}},{props:{variant:"buffer"},style:{zIndex:1,transition:`transform .${De}s linear`}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:ir||{animation:`${Oe} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite`}}]}))),gr=O("span",{name:"MuiLinearProgress",slot:"Bar2",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar2,n[`barColor${P(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar2Indeterminate,o.variant==="buffer"&&n.bar2Buffer]}})(B(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n},style:{"--LinearProgressBar2-barColor":(e.vars||e).palette[n].main}})),{props:({ownerState:n})=>n.variant!=="buffer"&&n.color!=="inherit",style:{backgroundColor:"var(--LinearProgressBar2-barColor, currentColor)"}},{props:({ownerState:n})=>n.variant!=="buffer"&&n.color==="inherit",style:{backgroundColor:"currentColor"}},{props:{color:"inherit"},style:{opacity:.3}},...Object.entries(e.palette).filter(K()).map(([n])=>({props:{color:n,variant:"buffer"},style:{backgroundColor:qe(e,n),transition:`transform .${De}s linear`}})),{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:cr||{animation:`${Ve} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite`}}]}))),fr=m.forwardRef(function(n,o){const r=Y({props:n,name:"MuiLinearProgress"}),{className:s,color:a="primary",value:d,valueBuffer:l,variant:f="indeterminate",...u}=r,c={...r,color:a,variant:f},p=dr(c),C=Ko(),v={},i={bar1:{},bar2:{}};if((f==="determinate"||f==="buffer")&&d!==void 0){v["aria-valuenow"]=Math.round(d),v["aria-valuemin"]=0,v["aria-valuemax"]=100;let g=d-100;C&&(g=-g),i.bar1.transform=`translateX(${g}%)`}if(f==="buffer"&&l!==void 0){let g=(l||0)-100;C&&(g=-g),i.bar2.transform=`translateX(${g}%)`}return t.jsxs(ur,{className:Ge(p.root,s),ownerState:c,role:"progressbar",...v,ref:o,...u,children:[f==="buffer"?t.jsx(pr,{className:p.dashed,ownerState:c}):null,t.jsx(mr,{className:p.bar1,ownerState:c,style:i.bar1}),f==="determinate"?null:t.jsx(gr,{className:p.bar2,ownerState:c,style:i.bar2})]})});function hr(e={}){const{autoHideDuration:n=null,disableWindowBlurListener:o=!1,onClose:r,open:s,resumeHideDuration:a}=e,d=Oo();m.useEffect(()=>{if(!s)return;function h(x){x.defaultPrevented||x.key==="Escape"&&(r==null||r(x,"escapeKeyDown"))}return document.addEventListener("keydown",h),()=>{document.removeEventListener("keydown",h)}},[s,r]);const l=Le((h,x)=>{r==null||r(h,x)}),f=Le(h=>{!r||h==null||d.start(h,()=>{l(null,"timeout")})});m.useEffect(()=>(s&&f(n),d.clear),[s,n,f,d]);const u=h=>{r==null||r(h,"clickaway")},c=d.clear,p=m.useCallback(()=>{n!=null&&f(a??n*.5)},[n,a,f]),C=h=>x=>{const b=h.onBlur;b==null||b(x),p()},v=h=>x=>{const b=h.onFocus;b==null||b(x),c()},i=h=>x=>{const b=h.onMouseEnter;b==null||b(x),c()},g=h=>x=>{const b=h.onMouseLeave;b==null||b(x),p()};return m.useEffect(()=>{if(!o&&s)return window.addEventListener("focus",p),window.addEventListener("blur",c),()=>{window.removeEventListener("focus",p),window.removeEventListener("blur",c)}},[o,s,p,c]),{getRootProps:(h={})=>{const x={...on(e),...on(h)};return{role:"presentation",...h,...x,onBlur:C(x),onFocus:v(x),onMouseEnter:i(x),onMouseLeave:g(x)}},onClickAway:u}}function br(e){return we("MuiSnackbarContent",e)}ke("MuiSnackbarContent",["root","message","action"]);const yr=e=>{const{classes:n}=e;return Ee({root:["root"],action:["action"],message:["message"]},br,n)},Cr=O(Vo,{name:"MuiSnackbarContent",slot:"Root"})(B(({theme:e})=>{const n=e.palette.mode==="light"?.8:.98;return{...e.typography.body2,color:e.vars?e.vars.palette.SnackbarContent.color:e.palette.getContrastText(rn(e.palette.background.default,n)),backgroundColor:e.vars?e.vars.palette.SnackbarContent.bg:rn(e.palette.background.default,n),display:"flex",alignItems:"center",flexWrap:"wrap",padding:"6px 16px",flexGrow:1,[e.breakpoints.up("sm")]:{flexGrow:"initial",minWidth:288}}})),xr=O("div",{name:"MuiSnackbarContent",slot:"Message"})({padding:"8px 0"}),vr=O("div",{name:"MuiSnackbarContent",slot:"Action"})({display:"flex",alignItems:"center",marginLeft:"auto",paddingLeft:16,marginRight:-8}),Sr=m.forwardRef(function(n,o){const r=Y({props:n,name:"MuiSnackbarContent"}),{action:s,className:a,message:d,role:l="alert",...f}=r,u=r,c=yr(u);return t.jsxs(Cr,{role:l,elevation:6,className:Ge(c.root,a),ownerState:u,ref:o,...f,children:[t.jsx(xr,{className:c.message,ownerState:u,children:d}),s?t.jsx(vr,{className:c.action,ownerState:u,children:s}):null]})});function jr(e){return we("MuiSnackbar",e)}ke("MuiSnackbar",["root","anchorOriginTopCenter","anchorOriginBottomCenter","anchorOriginTopRight","anchorOriginBottomRight","anchorOriginTopLeft","anchorOriginBottomLeft"]);const wr=e=>{const{classes:n,anchorOrigin:o}=e,r={root:["root",`anchorOrigin${P(o.vertical)}${P(o.horizontal)}`]};return Ee(r,jr,n)},kr=O("div",{name:"MuiSnackbar",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`anchorOrigin${P(o.anchorOrigin.vertical)}${P(o.anchorOrigin.horizontal)}`]]}})(B(({theme:e})=>({zIndex:(e.vars||e).zIndex.snackbar,position:"fixed",display:"flex",left:8,right:8,justifyContent:"center",alignItems:"center",variants:[{props:({ownerState:n})=>n.anchorOrigin.vertical==="top",style:{top:8,[e.breakpoints.up("sm")]:{top:24}}},{props:({ownerState:n})=>n.anchorOrigin.vertical!=="top",style:{bottom:8,[e.breakpoints.up("sm")]:{bottom:24}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="left",style:{justifyContent:"flex-start",[e.breakpoints.up("sm")]:{left:24,right:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="right",style:{justifyContent:"flex-end",[e.breakpoints.up("sm")]:{right:24,left:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="center",style:{[e.breakpoints.up("sm")]:{left:"50%",right:"auto",transform:"translateX(-50%)"}}}]}))),Er=m.forwardRef(function(n,o){const r=Y({props:n,name:"MuiSnackbar"}),s=zo(),a={enter:s.transitions.duration.enteringScreen,exit:s.transitions.duration.leavingScreen},{action:d,anchorOrigin:{vertical:l,horizontal:f}={vertical:"bottom",horizontal:"left"},autoHideDuration:u=null,children:c,className:p,ClickAwayListenerProps:C,ContentProps:v,disableWindowBlurListener:i=!1,message:g,onBlur:k,onClose:h,onFocus:x,onMouseEnter:b,onMouseLeave:z,open:$,resumeHideDuration:U,slots:y={},slotProps:E={},TransitionComponent:M,transitionDuration:W=a,TransitionProps:{onEnter:q,onExited:Xe,...Ht}={},...Nt}=r,G={...r,anchorOrigin:{vertical:l,horizontal:f},autoHideDuration:u,disableWindowBlurListener:i,TransitionComponent:M,transitionDuration:W},Wt=wr(G),{getRootProps:qt,onClickAway:Zt}=hr({...G}),[Kt,Je]=m.useState(!0),Yt=R=>{Je(!0),Xe&&Xe(R)},Xt=(R,_)=>{Je(!1),q&&q(R,_)},J={slots:{transition:M,...y},slotProps:{content:v,clickAwayListener:C,transition:Ht,...E}},[Jt,Qt]=Q("root",{ref:o,className:[Wt.root,p],elementType:kr,getSlotProps:qt,externalForwardedProps:{...J,...Nt},ownerState:G}),[eo,{ownerState:no,...to}]=Q("clickAwayListener",{elementType:er,externalForwardedProps:J,getSlotProps:R=>({onClickAway:(..._)=>{var Qe;const D=_[0];(Qe=R.onClickAway)==null||Qe.call(R,..._),!(D!=null&&D.defaultMuiPrevented)&&Zt(..._)}}),ownerState:G}),[oo,ro]=Q("content",{elementType:Sr,shouldForwardComponentProp:!0,externalForwardedProps:J,additionalProps:{message:g,action:d},ownerState:G}),[so,ao]=Q("transition",{elementType:Po,externalForwardedProps:J,getSlotProps:R=>({onEnter:(..._)=>{var D;(D=R.onEnter)==null||D.call(R,..._),Xt(..._)},onExited:(..._)=>{var D;(D=R.onExited)==null||D.call(R,..._),Yt(..._)}}),additionalProps:{appear:!0,in:$,timeout:W,direction:l==="top"?"down":"up"},ownerState:G});return!$&&Kt?null:t.jsx(eo,{...to,...y.clickAwayListener&&{ownerState:no},children:t.jsx(Jt,{...Qt,children:t.jsx(so,{...ao,children:c||t.jsx(oo,{...ro})})})})}),Tr=Lt(t.jsx("path",{d:"M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20z"})),Ar=Lt(t.jsx("path",{d:"M6 2v6h.01L6 8.01 10 12l-4 4 .01.01H6V22h12v-5.99h-.01L18 16l-4-4 4-3.99-.01-.01H18V2zm10 14.5V20H8v-3.5l4-4zm-4-5-4-4V4h8v3.5z"}));/**
 * React Router DOM v6.30.1
 *
 * Copyright (c) Remix Software Inc.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE.md file in the root directory of this source tree.
 *
 * @license MIT
 */function $e(){return $e=Object.assign?Object.assign.bind():function(e){for(var n=1;n<arguments.length;n++){var o=arguments[n];for(var r in o)Object.prototype.hasOwnProperty.call(o,r)&&(e[r]=o[r])}return e},$e.apply(this,arguments)}function Rr(e,n){if(e==null)return{};var o={},r=Object.keys(e),s,a;for(a=0;a<r.length;a++)s=r[a],!(n.indexOf(s)>=0)&&(o[s]=e[s]);return o}function Pr(e){return!!(e.metaKey||e.altKey||e.ctrlKey||e.shiftKey)}function Ir(e,n){return e.button===0&&(!n||n==="_self")&&!Pr(e)}function Fe(e){return e===void 0&&(e=""),new URLSearchParams(typeof e=="string"||Array.isArray(e)||e instanceof URLSearchParams?e:Object.keys(e).reduce((n,o)=>{let r=e[o];return n.concat(Array.isArray(r)?r.map(s=>[o,s]):[[o,r]])},[]))}function Lr(e,n){let o=Fe(e);return n&&n.forEach((r,s)=>{o.has(s)||n.getAll(s).forEach(a=>{o.append(s,a)})}),o}const _r=["onClick","relative","reloadDocument","replace","state","target","to","preventScrollReset","viewTransition"],Mr="6";try{window.__reactRouterVersion=Mr}catch{}const Dr=typeof window<"u"&&typeof window.document<"u"&&typeof window.document.createElement<"u",Or=/^(?:[a-z][a-z0-9+.-]*:|\/\/)/i,Ft=m.forwardRef(function(n,o){let{onClick:r,relative:s,reloadDocument:a,replace:d,state:l,target:f,to:u,preventScrollReset:c,viewTransition:p}=n,C=Rr(n,_r),{basename:v}=m.useContext(Fo),i,g=!1;if(typeof u=="string"&&Or.test(u)&&(i=u,Dr))try{let b=new URL(window.location.href),z=u.startsWith("//")?new URL(b.protocol+u):new URL(u),$=Bo(z.pathname,v);z.origin===b.origin&&$!=null?u=$+z.search+z.hash:g=!0}catch{}let k=Uo(u,{relative:s}),h=Vr(u,{replace:d,state:l,target:f,preventScrollReset:c,relative:s,viewTransition:p});function x(b){r&&r(b),b.defaultPrevented||h(b)}return m.createElement("a",$e({},C,{href:i||k,onClick:g||a?r:x,ref:o,target:f}))});var ln;(function(e){e.UseScrollRestoration="useScrollRestoration",e.UseSubmit="useSubmit",e.UseSubmitFetcher="useSubmitFetcher",e.UseFetcher="useFetcher",e.useViewTransitionState="useViewTransitionState"})(ln||(ln={}));var dn;(function(e){e.UseFetcher="useFetcher",e.UseFetchers="useFetchers",e.UseScrollRestoration="useScrollRestoration"})(dn||(dn={}));function Vr(e,n){let{target:o,replace:r,state:s,preventScrollReset:a,relative:d,viewTransition:l}=n===void 0?{}:n,f=Dt(),u=Mt(),c=Go(e,{relative:d});return m.useCallback(p=>{if(Ir(p,o)){p.preventDefault();let C=r!==void 0?r:sn(u)===sn(c);f(e,{replace:C,state:s,preventScrollReset:a,relative:d,viewTransition:l})}},[u,f,c,r,s,o,e,a,d,l])}function zr(e){let n=m.useRef(Fe(e)),o=m.useRef(!1),r=Mt(),s=m.useMemo(()=>Lr(r.search,o.current?null:n.current),[r.search]),a=Dt(),d=m.useCallback((l,f)=>{const u=Fe(typeof l=="function"?l(s):l);o.current=!0,a("?"+u,f)},[a,s]);return[s,d]}const Bt=()=>{const e=Be(),n=S(io),[o,r]=m.useState(null),s=!!o,a=S(co),d=c=>{r(c.currentTarget)},l=()=>{r(null)},f=()=>{l(),e(lo())},u=a.length==0?"":a.map((c,p)=>t.jsx(Er,{open:!0,autoHideDuration:6e3,onClose:()=>en(p),anchorOrigin:{vertical:"bottom",horizontal:"center"},children:t.jsx(N,{onClose:()=>en(p),severity:c.type,sx:{width:"100%"},children:c.message})},c.id));return t.jsxs(T,{sx:{display:"flex",flexDirection:"column",minHeight:"100vh",bgcolor:"background.default"},children:[n&&t.jsxs(We,{maxWidth:"lg",sx:{display:"flex",justifyContent:"flex-end",pt:{xs:1,sm:2}},children:[t.jsx(Z,{id:"account-button",onClick:d,color:"inherit",size:"small",endIcon:t.jsx(Io,{alt:n,src:"/assets/avatar.png",sx:{width:28,height:28,fontSize:14}}),"aria-controls":s?"account-menu":void 0,"aria-haspopup":"true","aria-expanded":s?"true":void 0,sx:{color:"text.secondary",minWidth:0,textTransform:"none"},children:t.jsx(w,{variant:"body2",component:"span",noWrap:!0,sx:{display:{xs:"none",sm:"inline"},maxWidth:260},children:n})}),t.jsxs(Lo,{id:"account-menu",anchorEl:o,open:s,onClose:l,MenuListProps:{"aria-labelledby":"account-button"},children:[t.jsx(tn,{disabled:!0,children:t.jsx(w,{variant:"body2",children:n})}),t.jsx(tn,{onClick:f,children:"Logout"})]})]}),t.jsxs(T,{component:"main",sx:{flexGrow:1},children:[t.jsx(Ho,{}),u]})]})};Bt.__docgenInfo={description:"Layout component for the application",methods:[],displayName:"Layout"};const Ze=({currentFormStep:e,formSubmitError:n,isSubmittingForm:o,onCancel:r,onSubmit:s})=>t.jsxs(Re,{open:e!==null,onClose:r,maxWidth:"sm",fullWidth:!0,children:[t.jsx(Pe,{sx:{pb:1},children:t.jsxs(T,{sx:{display:"flex",flexDirection:"column",gap:j.spacing.cardActionGap},children:[t.jsx(w,{variant:"h6",component:"span",children:"Complete setup"}),t.jsx(w,{variant:"body2",color:"text.secondary",children:(e==null?void 0:e.stepTitle)??"Provide the details needed to finish this connection."})]})}),t.jsxs(Ie,{dividers:!0,children:[n&&t.jsxs(N,{severity:"error",sx:{mb:2},children:[t.jsx(zt,{children:"Setup could not be saved"}),n]}),e&&t.jsx(Wo,{connectionId:e.connectionId,stepTitle:e.stepTitle,stepDescription:e.stepDescription,jsonSchema:e.jsonSchema,uiSchema:e.uiSchema,onSubmit:s,onCancel:r,isSubmitting:o})]})]});Ze.__docgenInfo={description:"",methods:[],displayName:"ConnectionSetupDialog",props:{currentFormStep:{required:!0,tsType:{name:"union",raw:"SetupStep | null",elements:[{name:"SetupStep"},{name:"null"}]},description:""},formSubmitError:{required:!0,tsType:{name:"union",raw:"string | null",elements:[{name:"string"},{name:"null"}]},description:""},isSubmittingForm:{required:!0,tsType:{name:"boolean"},description:""},onCancel:{required:!0,tsType:{name:"signature",type:"function",raw:"() => void",signature:{arguments:[],return:{name:"void"}}},description:""},onSubmit:{required:!0,tsType:{name:"signature",type:"function",raw:"(connectionId: string, data: unknown) => void",signature:{arguments:[{type:{name:"string"},name:"connectionId"},{type:{name:"unknown"},name:"data"}],return:{name:"void"}}},description:""}}};const Ut=()=>{const e=Be(),n=S(xt),o=S(vt),r=S(St),s=S(jt),a=S(wt),d=S(kt),l=S(Et);m.useEffect(()=>{o==="idle"&&e(Tt())},[o,e]);const f=C=>{const v=`${window.location.origin}/connections`;e(It({connectorId:C,returnToUrl:v})).then(i=>{if(i.meta.requestStatus==="fulfilled"){const g=i.payload;Se(g)&&(window.location.href=g.redirect_url)}})},u=m.useCallback((C,v)=>{const i=(a==null?void 0:a.stepId)??"";e(At({connectionId:C,stepId:i,data:v})).then(g=>{if(g.meta.requestStatus==="fulfilled"){const k=g.payload;Se(k)&&(window.location.href=k.redirect_url)}})},[e,a]),c=m.useCallback(()=>{e(a?Rt(a.connectionId):Pt())},[e,a]);let p;return o==="loading"?p=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(C=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Vt,{})},C))}):o==="failed"?p=t.jsx(N,{severity:"error",children:r}):n.length===0?p=t.jsx(T,{sx:{textAlign:"center",py:j.spacing.pageY},children:t.jsx(w,{variant:"h6",color:"text.secondary",children:"No connectors available"})}):p=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:n.map(C=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Ot,{connector:C,onConnect:f,isConnecting:s})},C.id))}),t.jsxs(We,{sx:{py:j.spacing.pageY},children:[t.jsxs(T,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:j.spacing.headerGap,mb:j.spacing.sectionGap},children:[t.jsx(w,{variant:"h4",component:"h1",children:"Available Connectors"}),t.jsxs(T,{sx:{display:"flex",alignItems:"center",gap:j.spacing.headerGap},children:[s&&t.jsxs(T,{sx:{display:"flex",alignItems:"center"},children:[t.jsx(_e,{size:24,sx:{mr:1}}),t.jsx(w,{variant:"body2",color:"text.secondary",children:"Connecting..."})]}),t.jsx(Z,{component:Ft,to:"/connections",startIcon:t.jsx(Tr,{}),sx:{alignSelf:{xs:"flex-start",sm:"center"}},children:"Back to Connections"})]})]}),p,t.jsx(Ze,{currentFormStep:a,formSubmitError:l,isSubmittingForm:d,onCancel:c,onSubmit:u})]})};Ut.__docgenInfo={description:"Component to display a list of available connectors",methods:[],displayName:"ConnectorList"};const Gt=()=>{const e=Be(),[n,o]=zr(),r=S(uo),s=S(po),a=S(mo),d=S(xt),l=S(vt),f=S(St),u=S(jt),c=S(wt),p=S(kt),C=S(Et),v=S(go),i=S(fo),g=S(ho);m.useEffect(()=>{s==="idle"&&e(Te()),l==="idle"&&e(Tt())},[s,l,e]),m.useEffect(()=>{const y=n.get("setup"),E=n.get("connection_id");y==="pending"&&E&&(e(nn(E)),n.delete("setup"),n.delete("connection_id"),o(n,{replace:!0}))},[n,o,e]),m.useEffect(()=>{if(!v)return;const y=window.setInterval(()=>{e(nn(v))},2e3);return()=>window.clearInterval(y)},[v,e]);const k=m.useCallback((y,E)=>{const M=(c==null?void 0:c.stepId)??"";e(At({connectionId:y,stepId:M,data:E})).then(W=>{if(W.meta.requestStatus==="fulfilled"){const q=W.payload;Se(q)?window.location.href=q.redirect_url:e(Te())}})},[e,c]),h=m.useCallback(()=>{const y=c==null?void 0:c.connectionId,E=y?r.find(M=>M.id===y):void 0;E&&E.state===je.CONFIGURED&&e(bo(E.id)),e(Pt())},[e,c,r]),x=m.useCallback(()=>{i&&e(yo({connectionId:i.connectionId,returnToUrl:window.location.href})).then(y=>{if(y.meta.requestStatus==="fulfilled"){const E=y.payload;E.type==="redirect"&&E.redirect_url&&(window.location.href=E.redirect_url)}})},[e,i]),b=m.useCallback(()=>{i&&e(Rt(i.connectionId)).then(()=>{e(Co()),e(Te())})},[e,i]),z=m.useCallback(y=>{e(It({connectorId:y,returnToUrl:`${window.location.origin}/connections`})).then(E=>{if(E.meta.requestStatus==="fulfilled"){const M=E.payload;Se(M)&&(window.location.href=M.redirect_url)}})},[e]),$=()=>l==="loading"||l==="idle"?t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(y=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Vt,{})},`connector-skeleton-${y}`))}):l==="failed"?t.jsx(N,{severity:"error",children:f}):d.length===0?t.jsx(T,{sx:{py:3},children:t.jsx(w,{color:"text.secondary",children:"No connectors are available right now."})}):t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:d.map(y=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Ot,{connector:y,onConnect:z,isConnecting:u})},y.id))});let U;return s==="loading"?U=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:[1,2,3,4].map(y=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(vo,{})},`connection-skeleton-${y}`))}):s==="failed"?U=t.jsx(N,{severity:"error",children:a}):r.length===0?U=t.jsxs(t.Fragment,{children:[t.jsxs(T,{sx:{border:1,borderColor:j.card.borderColor,borderRadius:j.radius.panel,bgcolor:j.card.surface,mb:j.spacing.sectionGap,p:j.spacing.panelPadding},children:[t.jsx(w,{variant:"h5",component:"h2",gutterBottom:!0,children:"Connect your first application"}),t.jsx(w,{color:"text.secondary",sx:{maxWidth:680},children:"Choose a connector below to create a connection. Once connected, it will appear here for ongoing setup, health, and management."}),u&&t.jsxs(T,{sx:{display:"flex",alignItems:"center",mt:3},children:[t.jsx(_e,{size:24,sx:{mr:1}}),t.jsx(w,{variant:"body2",color:"text.secondary",children:"Starting connection..."})]})]}),t.jsxs(T,{children:[t.jsx(w,{variant:"h6",component:"h2",sx:{mb:2},children:"Available connectors"}),$()]})]}):U=t.jsx(L,{container:!0,spacing:j.spacing.gridGap,children:r.map(y=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(xo,{connection:y})},y.id))}),t.jsxs(We,{sx:{py:j.spacing.pageY},children:[t.jsxs(T,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:j.spacing.headerGap,mb:j.spacing.sectionGap},children:[t.jsx(w,{variant:"h4",component:"h1",children:"Your Connections"}),r.length>0&&t.jsx(Z,{variant:"contained",color:"primary",startIcon:t.jsx(qo,{}),component:Ft,to:"/connectors",children:"Connect More"})]}),U,t.jsx(Ze,{currentFormStep:c,formSubmitError:C,isSubmittingForm:p,onCancel:h,onSubmit:k}),t.jsxs(Re,{open:v!==null,maxWidth:"xs",fullWidth:!0,children:[t.jsx(Pe,{sx:{pb:1},children:"Verifying connection"}),t.jsx(Ie,{dividers:!0,children:t.jsxs(T,{sx:{display:"flex",flexDirection:"column",alignItems:"center",gap:j.spacing.headerGap,py:3},children:[t.jsx(Ar,{color:"primary",sx:{fontSize:40}}),t.jsxs(T,{sx:{textAlign:"center"},children:[t.jsx(w,{variant:"subtitle1",component:"p",children:"Checking credentials"}),t.jsx(w,{variant:"body2",color:"text.secondary",children:"AuthProxy is confirming that this connection can reach the provider."})]}),t.jsx(fr,{sx:{width:"100%"}})]})})]}),t.jsxs(Re,{open:i!==null,onClose:b,maxWidth:"sm",fullWidth:!0,children:[t.jsx(Pe,{sx:{pb:1},children:t.jsxs(T,{sx:{display:"flex",alignItems:"center",gap:1},children:[t.jsx(Zo,{color:"error"}),t.jsx(w,{variant:"h6",component:"span",children:"Connection verification failed"})]})}),t.jsxs(Ie,{dividers:!0,children:[t.jsxs(N,{severity:"error",sx:{mb:2},children:[t.jsx(zt,{children:"Provider check failed"}),(i==null?void 0:i.message)??"Verification failed"]}),t.jsx(w,{variant:"body2",color:"text.secondary",children:i!=null&&i.canRetry?"Retry setup to run verification again. Cancel setup deletes this unfinished connection.":"Cancel setup to delete this unfinished connection, then start again from the connector."})]}),t.jsxs(_o,{children:[t.jsx(Z,{onClick:b,disabled:g,children:"Cancel setup"}),(i==null?void 0:i.canRetry)&&t.jsx(Z,{onClick:x,disabled:g,variant:"contained",startIcon:g?t.jsx(_e,{size:16}):void 0,children:g?"Retrying setup...":"Retry setup"})]})]})]})};Gt.__docgenInfo={description:"Component to display a list of connections",methods:[],displayName:"ConnectionList"};const H=(e,n,o="#ffffff")=>{const r=e.split(/\s+/).map(a=>a[0]).join("").slice(0,2).toUpperCase(),s=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${n}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="${o}" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">${r}</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(s)}`},V=[{id:"google-drive",namespace:"root",version:1,state:I.ACTIVE,display_name:"Google Drive",description:"Have the agent track your work in Google Drive.",highlight:"Have the agent track your work in Google Drive.",logo:H("Google Drive","#188038"),has_configure:!1,versions:1,states:[I.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"greenhouse",namespace:"root",version:1,state:I.ACTIVE,display_name:"Greenhouse",description:"This integration pushes candidates to greenhouse.",highlight:"This integration pushes candidates to greenhouse.",logo:H("Greenhouse","#24a47f"),has_configure:!1,versions:1,states:[I.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"google-calendar",namespace:"root",version:1,state:I.ACTIVE,display_name:"Google Calendar",description:"Allow the agent to manage your calendar on your behalf. It's like having your own personal assistant!!",highlight:"Allow the agent to manage your calendar on your behalf. It's like having your own personal assistant!!",logo:H("Google Calendar","#1a73e8"),has_configure:!0,versions:1,states:[I.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"gmail",namespace:"root",version:1,state:I.ACTIVE,display_name:"GMail",description:"Have the agent respond to your emails without you needing to be involved. Like magic.",highlight:"Have the agent respond to your emails without you needing to be involved. Like magic.",logo:H("GMail","#d93025"),has_configure:!1,versions:1,states:[I.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"pipedrive",namespace:"root",version:1,state:I.ACTIVE,display_name:"pipedrive",description:"Allow our agent to handle your sales support.",highlight:"Allow our agent to handle your sales support.",logo:H("pipedrive","#017a5e"),has_configure:!1,versions:1,states:[I.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"asana",namespace:"root",version:1,state:I.ACTIVE,display_name:"Asana",description:"Allow our agent organize your work.",highlight:"Allow our agent organize your work.",logo:H("Asana","#f06a6a"),has_configure:!1,versions:1,states:[I.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"}],F=(e,n={})=>({id:`cxn_${e.id}`,namespace:"root",connector:e,state:je.CONFIGURED,health_state:Ue.HEALTHY,created_at:"2024-04-01T12:00:00Z",updated_at:"2024-04-01T12:00:00Z",...n}),$r=[F(V[0]),F(V[2],{health_state:Ue.UNHEALTHY}),F(V[5],{state:je.SETUP}),F(V[4],{state:je.DISABLED})],Ke={connectionId:"cxn_google-calendar",stepId:"select-calendar",stepTitle:"Select a Calendar",stepDescription:"Choose which Google Calendar the agent should manage.",currentStep:0,totalSteps:2,jsonSchema:{type:"object",required:["calendar_id"],properties:{calendar_id:{type:"string",title:"Calendar",enum:["primary","product","support"]}}},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/calendar_id"}]}},A={items:$r,status:"succeeded",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null,currentFormStep:null,submittingForm:!1,formSubmitError:null,verifyingConnectionId:null,verifyError:null,retryingConnection:!1};function Fr({route:e,connectorsState:n={items:V,status:"succeeded",error:null},connectionsState:o=A}){const r=So({reducer:jo({auth:Ao,connectors:To,connections:Eo,toasts:ko}),preloadedState:{auth:{actor_id:"actor_storybook",status:"authenticated"},connectors:n,connections:o,toasts:{items:[]}}});return t.jsx(wo,{store:r,children:t.jsxs($o,{theme:Mo,children:[t.jsx(sr,{}),t.jsx(No,{children:t.jsx(an,{element:t.jsx(Bt,{}),children:t.jsx(an,{path:"*",element:e==="/connectors"?t.jsx(Ut,{}):t.jsx(Gt,{})})})})]})})}const ls={title:"Pages/Marketplace",component:Fr,parameters:{layout:"fullscreen"}},Ye={viewport:{defaultViewport:"marketplaceMobile"}},X={viewport:{defaultViewport:"marketplaceTablet"}},ee={args:{route:"/connectors"}},ne={args:{route:"/connectors",connectorsState:{items:[],status:"loading",error:null}}},te={args:{route:"/connections"}},oe={args:{route:"/connections"},parameters:Ye},re={args:{route:"/connections"},parameters:X},se={args:{route:"/connections",connectionsState:{...A,items:[F(V[2],{health_state:Ue.UNHEALTHY})]}}},ae={args:{route:"/connections",connectionsState:{...A,items:[F(V[2]),F(V[0])]}}},ie={args:{route:"/connections",connectionsState:{...A,items:[]}}},ce={args:{route:"/connections",connectionsState:{...A,items:[]}},parameters:Ye},le={args:{route:"/connections",connectionsState:{...A,items:[]}},parameters:X},de={args:{route:"/connections",connectorsState:{items:[],status:"loading",error:null},connectionsState:{...A,items:[]}}},ue={args:{route:"/connectors"},parameters:Ye},pe={args:{route:"/connectors"},parameters:X},me={args:{route:"/connections",connectionsState:{...A,currentFormStep:Ke}}},ge={args:{route:"/connections",connectionsState:{...A,currentFormStep:Ke}},parameters:X},fe={args:{route:"/connections",connectionsState:{...A,currentFormStep:Ke,submittingForm:!0}}},he={args:{route:"/connections",connectionsState:{...A,verifyingConnectionId:"cxn_google-calendar"}}},be={args:{route:"/connections",connectionsState:{...A,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}}},ye={args:{route:"/connections",connectionsState:{...A,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}},parameters:X},Ce={args:{route:"/connections",connectionsState:{...A,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0},retryingConnection:!0}}},xe={args:{route:"/connections",connectionsState:{...A,verifyError:{connectionId:"cxn_google-calendar",message:"The provider rejected this setup and it cannot be retried.",canRetry:!1}}}};var un,pn,mn;ee.parameters={...ee.parameters,docs:{...(un=ee.parameters)==null?void 0:un.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  }
}`,...(mn=(pn=ee.parameters)==null?void 0:pn.docs)==null?void 0:mn.source}}};var gn,fn,hn;ne.parameters={...ne.parameters,docs:{...(gn=ne.parameters)==null?void 0:gn.docs,source:{originalSource:`{
  args: {
    route: '/connectors',
    connectorsState: {
      items: [],
      status: 'loading',
      error: null
    }
  }
}`,...(hn=(fn=ne.parameters)==null?void 0:fn.docs)==null?void 0:hn.source}}};var bn,yn,Cn;te.parameters={...te.parameters,docs:{...(bn=te.parameters)==null?void 0:bn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  }
}`,...(Cn=(yn=te.parameters)==null?void 0:yn.docs)==null?void 0:Cn.source}}};var xn,vn,Sn;oe.parameters={...oe.parameters,docs:{...(xn=oe.parameters)==null?void 0:xn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: mobileViewport
}`,...(Sn=(vn=oe.parameters)==null?void 0:vn.docs)==null?void 0:Sn.source}}};var jn,wn,kn;re.parameters={...re.parameters,docs:{...(jn=re.parameters)==null?void 0:jn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: tabletViewport
}`,...(kn=(wn=re.parameters)==null?void 0:wn.docs)==null?void 0:kn.source}}};var En,Tn,An;se.parameters={...se.parameters,docs:{...(En=se.parameters)==null?void 0:En.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2], {
        health_state: ConnectionHealthState.UNHEALTHY
      })]
    }
  }
}`,...(An=(Tn=se.parameters)==null?void 0:Tn.docs)==null?void 0:An.source}}};var Rn,Pn,In;ae.parameters={...ae.parameters,docs:{...(Rn=ae.parameters)==null?void 0:Rn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2]), connectionFor(connectors[0])]
    }
  }
}`,...(In=(Pn=ae.parameters)==null?void 0:Pn.docs)==null?void 0:In.source}}};var Ln,_n,Mn;ie.parameters={...ie.parameters,docs:{...(Ln=ie.parameters)==null?void 0:Ln.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  }
}`,...(Mn=(_n=ie.parameters)==null?void 0:_n.docs)==null?void 0:Mn.source}}};var Dn,On,Vn;ce.parameters={...ce.parameters,docs:{...(Dn=ce.parameters)==null?void 0:Dn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: mobileViewport
}`,...(Vn=(On=ce.parameters)==null?void 0:On.docs)==null?void 0:Vn.source}}};var zn,$n,Fn;le.parameters={...le.parameters,docs:{...(zn=le.parameters)==null?void 0:zn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: tabletViewport
}`,...(Fn=($n=le.parameters)==null?void 0:$n.docs)==null?void 0:Fn.source}}};var Bn,Un,Gn;de.parameters={...de.parameters,docs:{...(Bn=de.parameters)==null?void 0:Bn.docs,source:{originalSource:`{
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
}`,...(Gn=(Un=de.parameters)==null?void 0:Un.docs)==null?void 0:Gn.source}}};var Hn,Nn,Wn;ue.parameters={...ue.parameters,docs:{...(Hn=ue.parameters)==null?void 0:Hn.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: mobileViewport
}`,...(Wn=(Nn=ue.parameters)==null?void 0:Nn.docs)==null?void 0:Wn.source}}};var qn,Zn,Kn;pe.parameters={...pe.parameters,docs:{...(qn=pe.parameters)==null?void 0:qn.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: tabletViewport
}`,...(Kn=(Zn=pe.parameters)==null?void 0:Zn.docs)==null?void 0:Kn.source}}};var Yn,Xn,Jn;me.parameters={...me.parameters,docs:{...(Yn=me.parameters)==null?void 0:Yn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  }
}`,...(Jn=(Xn=me.parameters)==null?void 0:Xn.docs)==null?void 0:Jn.source}}};var Qn,et,nt;ge.parameters={...ge.parameters,docs:{...(Qn=ge.parameters)==null?void 0:Qn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  },
  parameters: tabletViewport
}`,...(nt=(et=ge.parameters)==null?void 0:et.docs)==null?void 0:nt.source}}};var tt,ot,rt;fe.parameters={...fe.parameters,docs:{...(tt=fe.parameters)==null?void 0:tt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep,
      submittingForm: true
    }
  }
}`,...(rt=(ot=fe.parameters)==null?void 0:ot.docs)==null?void 0:rt.source}}};var st,at,it;he.parameters={...he.parameters,docs:{...(st=he.parameters)==null?void 0:st.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyingConnectionId: 'cxn_google-calendar'
    }
  }
}`,...(it=(at=he.parameters)==null?void 0:at.docs)==null?void 0:it.source}}};var ct,lt,dt;be.parameters={...be.parameters,docs:{...(ct=be.parameters)==null?void 0:ct.docs,source:{originalSource:`{
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
}`,...(dt=(lt=be.parameters)==null?void 0:lt.docs)==null?void 0:dt.source}}};var ut,pt,mt;ye.parameters={...ye.parameters,docs:{...(ut=ye.parameters)==null?void 0:ut.docs,source:{originalSource:`{
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
}`,...(mt=(pt=ye.parameters)==null?void 0:pt.docs)==null?void 0:mt.source}}};var gt,ft,ht;Ce.parameters={...Ce.parameters,docs:{...(gt=Ce.parameters)==null?void 0:gt.docs,source:{originalSource:`{
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
}`,...(ht=(ft=Ce.parameters)==null?void 0:ft.docs)==null?void 0:ht.source}}};var bt,yt,Ct;xe.parameters={...xe.parameters,docs:{...(bt=xe.parameters)==null?void 0:bt.docs,source:{originalSource:`{
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
}`,...(Ct=(yt=xe.parameters)==null?void 0:yt.docs)==null?void 0:Ct.source}}};const ds=["AvailableConnectors","AvailableConnectorsLoading","ConnectionsPopulated","ConnectionsPopulatedMobile","ConnectionsPopulatedTablet","ConnectionsNeedsAttention","ConnectionsHealthyActions","ConnectionsEmpty","ConnectionsEmptyMobile","ConnectionsEmptyTablet","ConnectionsEmptyLoadingConnectors","AvailableConnectorsMobile","AvailableConnectorsTablet","ConnectionSetupDialog","ConnectionSetupDialogTablet","ConnectionSetupSubmitting","VerifyingConnectionDialog","VerificationFailedDialog","VerificationFailedDialogTablet","VerificationRetryingDialog","VerificationFailedNoRetryDialog"];export{ee as AvailableConnectors,ne as AvailableConnectorsLoading,ue as AvailableConnectorsMobile,pe as AvailableConnectorsTablet,me as ConnectionSetupDialog,ge as ConnectionSetupDialogTablet,fe as ConnectionSetupSubmitting,ie as ConnectionsEmpty,de as ConnectionsEmptyLoadingConnectors,ce as ConnectionsEmptyMobile,le as ConnectionsEmptyTablet,ae as ConnectionsHealthyActions,se as ConnectionsNeedsAttention,te as ConnectionsPopulated,oe as ConnectionsPopulatedMobile,re as ConnectionsPopulatedTablet,be as VerificationFailedDialog,ye as VerificationFailedDialogTablet,xe as VerificationFailedNoRetryDialog,Ce as VerificationRetryingDialog,he as VerifyingConnectionDialog,ds as __namedExportsOrder,ls as default};

import{j as t}from"./jsx-runtime-BjG_zV1W.js";import{u as K,e as j,s as vo,f as So,g as dn,t as jo,h as Dt,i as Ft,j as Ot,k as Vt,l as zt,m as Ut,n as $t,o as Bt,p as Ze,q as Ye,r as Ke,v as Xe,w as wo,x as ko,y as To,z as Eo,A as Ao,B as Ro,D as Me,E as un,F as Io,G as Po,H as Lo,C as _o,a as Mo,c as Do,I as Fo,P as Oo,J as Vo,b as zo,d as Uo,K as $o}from"./ConnectionCard-BQz_O3lE.js";import"./client-Bd8K-UT9.js";import{M as Bo,d as Go,r as No,S as De,C as P}from"./index-C7dhk8kh.js";import{g as Ho,o as Fe,G as Wo,A as qo,M as Zo,b as pn,D as Oe,c as Ve,d as ze,i as ke,a as Te,e as Yo,C as Je}from"./connections-By4y3SDg.js";import{m as f,t as Ko}from"./theme-D4YlWSlu.js";import{r as p}from"./index-yIsmwZOr.js";import{b as Xo,e as Ue,f as Jo,g as mn,P as Gt,c as ee,B}from"./Button-Ct-3TB0V.js";import{T as y,g as Nt,u as Qo}from"./Typography-zR6YQtWt.js";import{i as Ae,l as Re,u as X,s as F,a as Qe,g as Ie,o as G,n as I,p as Y,A as en,B as nn,K as gn,L as er}from"./createSimplePaletteValueFilter-DRANIdZ4.js";import{A as V,C as Pe}from"./Container-C8o4Yvv6.js";import{B as v,C as Ee}from"./Box-qKFn2-pG.js";import{N as nr,s as tr,a as or,b as Ht,u as tn,c as rr,d as fn,O as sr,e as ar,R as ir,f as hn}from"./index-ujyxIXwO.js";import"./index-M3uX8AIl.js";import{c as Wt}from"./createSvgIcon-YAXHmVIe.js";import{C as cr,D as lr,G as L,A as dr,E as ur}from"./ConnectionFormStep-DsEyO4q4.js";import{C as qt,a as Zt}from"./ConnectorCard-BsGxnjR4.js";import{u as pr}from"./Chip-oJjan0S7.js";import"./IconButton-CLik2w7b.js";import"./Close-hpK41xwR.js";import"./useThemeProps-oZcovqdW.js";import"./Stack-Cf6NjqW7.js";function mr(e){return Ae("MuiAlertTitle",e)}Re("MuiAlertTitle",["root"]);const gr=e=>{const{classes:n}=e;return Ie({root:["root"]},mr,n)},fr=F(y,{name:"MuiAlertTitle",slot:"Root"})(G(({theme:e})=>({fontWeight:e.typography.fontWeightMedium,marginTop:-2}))),Yt=p.forwardRef(function(n,o){const r=X({props:n,name:"MuiAlertTitle"}),{className:a,...i}=r,u=r,l=gr(u);return t.jsx(fr,{gutterBottom:!0,component:"div",ownerState:u,ref:o,className:Qe(l.root,a),...i})});function bn(e){return e.substring(2).toLowerCase()}function hr(e,n){return n.documentElement.clientWidth<e.clientX||n.documentElement.clientHeight<e.clientY}function br(e){const{children:n,disableReactTree:o=!1,mouseEvent:r="onClick",onClickAway:a,touchEvent:i="onTouchEnd"}=e,u=p.useRef(!1),l=p.useRef(null),g=p.useRef(!1),d=p.useRef(!1);p.useEffect(()=>(setTimeout(()=>{g.current=!0},0),()=>{g.current=!1}),[]);const c=Xo(Ho(n),l),m=Ue(s=>{const h=d.current;d.current=!1;const T=Fe(l.current);if(!g.current||!l.current||"clientX"in s&&hr(s,T))return;if(u.current){u.current=!1;return}let b;s.composedPath?b=s.composedPath().includes(l.current):b=!T.documentElement.contains(s.target)||l.current.contains(s.target),!b&&(o||!h)&&a(s)}),k=s=>h=>{d.current=!0;const T=n.props[s];T&&T(h)},x={ref:c};return i!==!1&&(x[i]=k(i)),p.useEffect(()=>{if(i!==!1){const s=bn(i),h=Fe(l.current),T=()=>{u.current=!0};return h.addEventListener(s,m),h.addEventListener("touchmove",T),()=>{h.removeEventListener(s,m),h.removeEventListener("touchmove",T)}}},[m,i]),r!==!1&&(x[r]=k(r)),p.useEffect(()=>{if(r!==!1){const s=bn(r),h=Fe(l.current);return h.addEventListener(s,m),()=>{h.removeEventListener(s,m)}}},[m,r]),p.cloneElement(n,x)}const $e=typeof Nt({})=="function",xr=(e,n)=>({WebkitFontSmoothing:"antialiased",MozOsxFontSmoothing:"grayscale",boxSizing:"border-box",WebkitTextSizeAdjust:"100%",...n&&!e.vars&&{colorScheme:e.palette.mode}}),yr=e=>({color:(e.vars||e).palette.text.primary,...e.typography.body1,backgroundColor:(e.vars||e).palette.background.default,"@media print":{backgroundColor:(e.vars||e).palette.common.white}}),Kt=(e,n=!1)=>{var i,u;const o={};n&&e.colorSchemes&&typeof e.getColorSchemeSelector=="function"&&Object.entries(e.colorSchemes).forEach(([l,g])=>{var c,m;const d=e.getColorSchemeSelector(l);d.startsWith("@")?o[d]={":root":{colorScheme:(c=g.palette)==null?void 0:c.mode}}:o[d.replace(/\s*&/,"")]={colorScheme:(m=g.palette)==null?void 0:m.mode}});let r={html:xr(e,n),"*, *::before, *::after":{boxSizing:"inherit"},"strong, b":{fontWeight:e.typography.fontWeightBold},body:{margin:0,...yr(e),"&::backdrop":{backgroundColor:(e.vars||e).palette.background.default}},...o};const a=(u=(i=e.components)==null?void 0:i.MuiCssBaseline)==null?void 0:u.styleOverrides;return a&&(r=[r,a]),r},we="mui-ecs",Cr=e=>{const n=Kt(e,!1),o=Array.isArray(n)?n[0]:n;return!e.vars&&o&&(o.html[`:root:has(${we})`]={colorScheme:e.palette.mode}),e.colorSchemes&&Object.entries(e.colorSchemes).forEach(([r,a])=>{var u,l;const i=e.getColorSchemeSelector(r);i.startsWith("@")?o[i]={[`:root:not(:has(.${we}))`]:{colorScheme:(u=a.palette)==null?void 0:u.mode}}:o[i.replace(/\s*&/,"")]={[`&:not(:has(.${we}))`]:{colorScheme:(l=a.palette)==null?void 0:l.mode}}}),n},vr=Nt($e?({theme:e,enableColorScheme:n})=>Kt(e,n):({theme:e})=>Cr(e));function Sr(e){const n=X({props:e,name:"MuiCssBaseline"}),{children:o,enableColorScheme:r=!1}=n;return t.jsxs(p.Fragment,{children:[$e&&t.jsx(vr,{enableColorScheme:r}),!$e&&!r&&t.jsx("span",{className:we,style:{display:"none"}}),o]})}function jr(e){return Ae("MuiLinearProgress",e)}Re("MuiLinearProgress",["root","colorPrimary","colorSecondary","determinate","indeterminate","buffer","query","dashed","dashedColorPrimary","dashedColorSecondary","bar","bar1","bar2","barColorPrimary","barColorSecondary","bar1Indeterminate","bar1Determinate","bar1Buffer","bar2Indeterminate","bar2Buffer"]);const Be=4,Ge=nn`
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
`,wr=typeof Ge!="string"?en`
        animation: ${Ge} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite;
      `:null,Ne=nn`
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
`,kr=typeof Ne!="string"?en`
        animation: ${Ne} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite;
      `:null,He=nn`
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
`,Tr=typeof He!="string"?en`
        animation: ${He} 3s infinite linear;
      `:null,Er=e=>{const{classes:n,variant:o,color:r}=e,a={root:["root",`color${I(r)}`,o],dashed:["dashed",`dashedColor${I(r)}`],bar1:["bar","bar1",`barColor${I(r)}`,(o==="indeterminate"||o==="query")&&"bar1Indeterminate",o==="determinate"&&"bar1Determinate",o==="buffer"&&"bar1Buffer"],bar2:["bar","bar2",o!=="buffer"&&`barColor${I(r)}`,o==="buffer"&&`color${I(r)}`,(o==="indeterminate"||o==="query")&&"bar2Indeterminate",o==="buffer"&&"bar2Buffer"]};return Ie(a,jr,n)},on=(e,n)=>e.vars?e.vars.palette.LinearProgress[`${n}Bg`]:e.palette.mode==="light"?e.lighten(e.palette[n].main,.62):e.darken(e.palette[n].main,.5),Ar=F("span",{name:"MuiLinearProgress",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`color${I(o.color)}`],n[o.variant]]}})(G(({theme:e})=>({position:"relative",overflow:"hidden",display:"block",height:4,zIndex:0,"@media print":{colorAdjust:"exact"},variants:[...Object.entries(e.palette).filter(Y()).map(([n])=>({props:{color:n},style:{backgroundColor:on(e,n)}})),{props:({ownerState:n})=>n.color==="inherit"&&n.variant!=="buffer",style:{"&::before":{content:'""',position:"absolute",left:0,top:0,right:0,bottom:0,backgroundColor:"currentColor",opacity:.3}}},{props:{variant:"buffer"},style:{backgroundColor:"transparent"}},{props:{variant:"query"},style:{transform:"rotate(180deg)"}}]}))),Rr=F("span",{name:"MuiLinearProgress",slot:"Dashed",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.dashed,n[`dashedColor${I(o.color)}`]]}})(G(({theme:e})=>({position:"absolute",marginTop:0,height:"100%",width:"100%",backgroundSize:"10px 10px",backgroundPosition:"0 -23px",variants:[{props:{color:"inherit"},style:{opacity:.3,backgroundImage:"radial-gradient(currentColor 0%, currentColor 16%, transparent 42%)"}},...Object.entries(e.palette).filter(Y()).map(([n])=>{const o=on(e,n);return{props:{color:n},style:{backgroundImage:`radial-gradient(${o} 0%, ${o} 16%, transparent 42%)`}}})]})),Tr||{animation:`${He} 3s infinite linear`}),Ir=F("span",{name:"MuiLinearProgress",slot:"Bar1",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar1,n[`barColor${I(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar1Indeterminate,o.variant==="determinate"&&n.bar1Determinate,o.variant==="buffer"&&n.bar1Buffer]}})(G(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[{props:{color:"inherit"},style:{backgroundColor:"currentColor"}},...Object.entries(e.palette).filter(Y()).map(([n])=>({props:{color:n},style:{backgroundColor:(e.vars||e).palette[n].main}})),{props:{variant:"determinate"},style:{transition:`transform .${Be}s linear`}},{props:{variant:"buffer"},style:{zIndex:1,transition:`transform .${Be}s linear`}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:wr||{animation:`${Ge} 2.1s cubic-bezier(0.65, 0.815, 0.735, 0.395) infinite`}}]}))),Pr=F("span",{name:"MuiLinearProgress",slot:"Bar2",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.bar,n.bar2,n[`barColor${I(o.color)}`],(o.variant==="indeterminate"||o.variant==="query")&&n.bar2Indeterminate,o.variant==="buffer"&&n.bar2Buffer]}})(G(({theme:e})=>({width:"100%",position:"absolute",left:0,bottom:0,top:0,transition:"transform 0.2s linear",transformOrigin:"left",variants:[...Object.entries(e.palette).filter(Y()).map(([n])=>({props:{color:n},style:{"--LinearProgressBar2-barColor":(e.vars||e).palette[n].main}})),{props:({ownerState:n})=>n.variant!=="buffer"&&n.color!=="inherit",style:{backgroundColor:"var(--LinearProgressBar2-barColor, currentColor)"}},{props:({ownerState:n})=>n.variant!=="buffer"&&n.color==="inherit",style:{backgroundColor:"currentColor"}},{props:{color:"inherit"},style:{opacity:.3}},...Object.entries(e.palette).filter(Y()).map(([n])=>({props:{color:n,variant:"buffer"},style:{backgroundColor:on(e,n),transition:`transform .${Be}s linear`}})),{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:{width:"auto"}},{props:({ownerState:n})=>n.variant==="indeterminate"||n.variant==="query",style:kr||{animation:`${Ne} 2.1s cubic-bezier(0.165, 0.84, 0.44, 1) 1.15s infinite`}}]}))),Lr=p.forwardRef(function(n,o){const r=X({props:n,name:"MuiLinearProgress"}),{className:a,color:i="primary",value:u,valueBuffer:l,variant:g="indeterminate",...d}=r,c={...r,color:i,variant:g},m=Er(c),k=pr(),x={},s={bar1:{},bar2:{}};if((g==="determinate"||g==="buffer")&&u!==void 0){x["aria-valuenow"]=Math.round(u),x["aria-valuemin"]=0,x["aria-valuemax"]=100;let h=u-100;k&&(h=-h),s.bar1.transform=`translateX(${h}%)`}if(g==="buffer"&&l!==void 0){let h=(l||0)-100;k&&(h=-h),s.bar2.transform=`translateX(${h}%)`}return t.jsxs(Ar,{className:Qe(m.root,a),ownerState:c,role:"progressbar",...x,ref:o,...d,children:[g==="buffer"?t.jsx(Rr,{className:m.dashed,ownerState:c}):null,t.jsx(Ir,{className:m.bar1,ownerState:c,style:s.bar1}),g==="determinate"?null:t.jsx(Pr,{className:m.bar2,ownerState:c,style:s.bar2})]})});function _r(e={}){const{autoHideDuration:n=null,disableWindowBlurListener:o=!1,onClose:r,open:a,resumeHideDuration:i}=e,u=Jo();p.useEffect(()=>{if(!a)return;function b(w){w.defaultPrevented||w.key==="Escape"&&(r==null||r(w,"escapeKeyDown"))}return document.addEventListener("keydown",b),()=>{document.removeEventListener("keydown",b)}},[a,r]);const l=Ue((b,w)=>{r==null||r(b,w)}),g=Ue(b=>{!r||b==null||u.start(b,()=>{l(null,"timeout")})});p.useEffect(()=>(a&&g(n),u.clear),[a,n,g,u]);const d=b=>{r==null||r(b,"clickaway")},c=u.clear,m=p.useCallback(()=>{n!=null&&g(i??n*.5)},[n,i,g]),k=b=>w=>{const C=b.onBlur;C==null||C(w),m()},x=b=>w=>{const C=b.onFocus;C==null||C(w),c()},s=b=>w=>{const C=b.onMouseEnter;C==null||C(w),c()},h=b=>w=>{const C=b.onMouseLeave;C==null||C(w),m()};return p.useEffect(()=>{if(!o&&a)return window.addEventListener("focus",m),window.addEventListener("blur",c),()=>{window.removeEventListener("focus",m),window.removeEventListener("blur",c)}},[o,a,m,c]),{getRootProps:(b={})=>{const w={...mn(e),...mn(b)};return{role:"presentation",...b,...w,onBlur:k(w),onFocus:x(w),onMouseEnter:s(w),onMouseLeave:h(w)}},onClickAway:d}}function Mr(e){return Ae("MuiSnackbarContent",e)}Re("MuiSnackbarContent",["root","message","action"]);const Dr=e=>{const{classes:n}=e;return Ie({root:["root"],action:["action"],message:["message"]},Mr,n)},Fr=F(Gt,{name:"MuiSnackbarContent",slot:"Root"})(G(({theme:e})=>{const n=e.palette.mode==="light"?.8:.98;return{...e.typography.body2,color:e.vars?e.vars.palette.SnackbarContent.color:e.palette.getContrastText(gn(e.palette.background.default,n)),backgroundColor:e.vars?e.vars.palette.SnackbarContent.bg:gn(e.palette.background.default,n),display:"flex",alignItems:"center",flexWrap:"wrap",padding:"6px 16px",flexGrow:1,[e.breakpoints.up("sm")]:{flexGrow:"initial",minWidth:288}}})),Or=F("div",{name:"MuiSnackbarContent",slot:"Message"})({padding:"8px 0"}),Vr=F("div",{name:"MuiSnackbarContent",slot:"Action"})({display:"flex",alignItems:"center",marginLeft:"auto",paddingLeft:16,marginRight:-8}),zr=p.forwardRef(function(n,o){const r=X({props:n,name:"MuiSnackbarContent"}),{action:a,className:i,message:u,role:l="alert",...g}=r,d=r,c=Dr(d);return t.jsxs(Fr,{role:l,elevation:6,className:Qe(c.root,i),ownerState:d,ref:o,...g,children:[t.jsx(Or,{className:c.message,ownerState:d,children:u}),a?t.jsx(Vr,{className:c.action,ownerState:d,children:a}):null]})});function Ur(e){return Ae("MuiSnackbar",e)}Re("MuiSnackbar",["root","anchorOriginTopCenter","anchorOriginBottomCenter","anchorOriginTopRight","anchorOriginBottomRight","anchorOriginTopLeft","anchorOriginBottomLeft"]);const $r=e=>{const{classes:n,anchorOrigin:o}=e,r={root:["root",`anchorOrigin${I(o.vertical)}${I(o.horizontal)}`]};return Ie(r,Ur,n)},Br=F("div",{name:"MuiSnackbar",slot:"Root",overridesResolver:(e,n)=>{const{ownerState:o}=e;return[n.root,n[`anchorOrigin${I(o.anchorOrigin.vertical)}${I(o.anchorOrigin.horizontal)}`]]}})(G(({theme:e})=>({zIndex:(e.vars||e).zIndex.snackbar,position:"fixed",display:"flex",left:8,right:8,justifyContent:"center",alignItems:"center",variants:[{props:({ownerState:n})=>n.anchorOrigin.vertical==="top",style:{top:8,[e.breakpoints.up("sm")]:{top:24}}},{props:({ownerState:n})=>n.anchorOrigin.vertical!=="top",style:{bottom:8,[e.breakpoints.up("sm")]:{bottom:24}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="left",style:{justifyContent:"flex-start",[e.breakpoints.up("sm")]:{left:24,right:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="right",style:{justifyContent:"flex-end",[e.breakpoints.up("sm")]:{right:24,left:"auto"}}},{props:({ownerState:n})=>n.anchorOrigin.horizontal==="center",style:{[e.breakpoints.up("sm")]:{left:"50%",right:"auto",transform:"translateX(-50%)"}}}]}))),Gr=p.forwardRef(function(n,o){const r=X({props:n,name:"MuiSnackbar"}),a=Qo(),i={enter:a.transitions.duration.enteringScreen,exit:a.transitions.duration.leavingScreen},{action:u,anchorOrigin:{vertical:l,horizontal:g}={vertical:"bottom",horizontal:"left"},autoHideDuration:d=null,children:c,className:m,ClickAwayListenerProps:k,ContentProps:x,disableWindowBlurListener:s=!1,message:h,onBlur:T,onClose:b,onFocus:w,onMouseEnter:C,onMouseLeave:z,open:U,resumeHideDuration:N,slots:S={},slotProps:E={},TransitionComponent:M,transitionDuration:q=i,TransitionProps:{onEnter:Z,onExited:an,...oo}={},...ro}=r,H={...r,anchorOrigin:{vertical:l,horizontal:g},autoHideDuration:d,disableWindowBlurListener:s,TransitionComponent:M,transitionDuration:q},so=$r(H),{getRootProps:ao,onClickAway:io}=_r({...H}),[co,cn]=p.useState(!0),lo=R=>{cn(!0),an&&an(R)},uo=(R,_)=>{cn(!1),Z&&Z(R,_)},Q={slots:{transition:M,...S},slotProps:{content:x,clickAwayListener:k,transition:oo,...E}},[po,mo]=ee("root",{ref:o,className:[so.root,m],elementType:Br,getSlotProps:ao,externalForwardedProps:{...Q,...ro},ownerState:H}),[go,{ownerState:fo,...ho}]=ee("clickAwayListener",{elementType:br,externalForwardedProps:Q,getSlotProps:R=>({onClickAway:(..._)=>{var ln;const D=_[0];(ln=R.onClickAway)==null||ln.call(R,..._),!(D!=null&&D.defaultMuiPrevented)&&io(..._)}}),ownerState:H}),[bo,xo]=ee("content",{elementType:zr,shouldForwardComponentProp:!0,externalForwardedProps:Q,additionalProps:{message:h,action:u},ownerState:H}),[yo,Co]=ee("transition",{elementType:Wo,externalForwardedProps:Q,getSlotProps:R=>({onEnter:(..._)=>{var D;(D=R.onEnter)==null||D.call(R,..._),uo(..._)},onExited:(..._)=>{var D;(D=R.onExited)==null||D.call(R,..._),lo(..._)}}),additionalProps:{appear:!0,in:U,timeout:q,direction:l==="top"?"down":"up"},ownerState:H});return!U&&co?null:t.jsx(go,{...ho,...S.clickAwayListener&&{ownerState:fo},children:t.jsx(po,{...mo,children:t.jsx(yo,{...Co,children:c||t.jsx(bo,{...xo})})})})}),Xt=Wt(t.jsx("path",{d:"M20 11H7.83l5.59-5.59L12 4l-8 8 8 8 1.41-1.41L7.83 13H20z"})),Nr=Wt(t.jsx("path",{d:"M6 2v6h.01L6 8.01 10 12l-4 4 .01.01H6V22h12v-5.99h-.01L18 16l-4-4 4-3.99-.01-.01H18V2zm10 14.5V20H8v-3.5l4-4zm-4-5-4-4V4h8v3.5z"}));/**
 * React Router DOM v6.30.1
 *
 * Copyright (c) Remix Software Inc.
 *
 * This source code is licensed under the MIT license found in the
 * LICENSE.md file in the root directory of this source tree.
 *
 * @license MIT
 */function We(){return We=Object.assign?Object.assign.bind():function(e){for(var n=1;n<arguments.length;n++){var o=arguments[n];for(var r in o)Object.prototype.hasOwnProperty.call(o,r)&&(e[r]=o[r])}return e},We.apply(this,arguments)}function Hr(e,n){if(e==null)return{};var o={},r=Object.keys(e),a,i;for(i=0;i<r.length;i++)a=r[i],!(n.indexOf(a)>=0)&&(o[a]=e[a]);return o}function Wr(e){return!!(e.metaKey||e.altKey||e.ctrlKey||e.shiftKey)}function qr(e,n){return e.button===0&&(!n||n==="_self")&&!Wr(e)}function qe(e){return e===void 0&&(e=""),new URLSearchParams(typeof e=="string"||Array.isArray(e)||e instanceof URLSearchParams?e:Object.keys(e).reduce((n,o)=>{let r=e[o];return n.concat(Array.isArray(r)?r.map(a=>[o,a]):[[o,r]])},[]))}function Zr(e,n){let o=qe(e);return n&&n.forEach((r,a)=>{o.has(a)||n.getAll(a).forEach(i=>{o.append(a,i)})}),o}const Yr=["onClick","relative","reloadDocument","replace","state","target","to","preventScrollReset","viewTransition"],Kr="6";try{window.__reactRouterVersion=Kr}catch{}const Xr=typeof window<"u"&&typeof window.document<"u"&&typeof window.document.createElement<"u",Jr=/^(?:[a-z][a-z0-9+.-]*:|\/\/)/i,rn=p.forwardRef(function(n,o){let{onClick:r,relative:a,reloadDocument:i,replace:u,state:l,target:g,to:d,preventScrollReset:c,viewTransition:m}=n,k=Hr(n,Yr),{basename:x}=p.useContext(nr),s,h=!1;if(typeof d=="string"&&Jr.test(d)&&(s=d,Xr))try{let C=new URL(window.location.href),z=d.startsWith("//")?new URL(C.protocol+d):new URL(d),U=tr(z.pathname,x);z.origin===C.origin&&U!=null?d=U+z.search+z.hash:h=!0}catch{}let T=or(d,{relative:a}),b=Qr(d,{replace:u,state:l,target:g,preventScrollReset:c,relative:a,viewTransition:m});function w(C){r&&r(C),C.defaultPrevented||b(C)}return p.createElement("a",We({},k,{href:s||T,onClick:h||i?r:w,ref:o,target:g}))});var xn;(function(e){e.UseScrollRestoration="useScrollRestoration",e.UseSubmit="useSubmit",e.UseSubmitFetcher="useSubmitFetcher",e.UseFetcher="useFetcher",e.useViewTransitionState="useViewTransitionState"})(xn||(xn={}));var yn;(function(e){e.UseFetcher="useFetcher",e.UseFetchers="useFetchers",e.UseScrollRestoration="useScrollRestoration"})(yn||(yn={}));function Qr(e,n){let{target:o,replace:r,state:a,preventScrollReset:i,relative:u,viewTransition:l}=n===void 0?{}:n,g=tn(),d=Ht(),c=rr(e,{relative:u});return p.useCallback(m=>{if(qr(m,o)){m.preventDefault();let k=r!==void 0?r:fn(d)===fn(c);g(e,{replace:k,state:a,preventScrollReset:i,relative:u,viewTransition:l})}},[d,g,c,r,a,o,e,i,u,l])}function es(e){let n=p.useRef(qe(e)),o=p.useRef(!1),r=Ht(),a=p.useMemo(()=>Zr(r.search,o.current?null:n.current),[r.search]),i=tn(),u=p.useCallback((l,g)=>{const d=qe(typeof l=="function"?l(a):l);o.current=!0,i("?"+d,g)},[i,a]);return[a,u]}const Jt=()=>{const e=K(),n=j(vo),[o,r]=p.useState(null),a=!!o,i=j(So),u=c=>{r(c.currentTarget)},l=()=>{r(null)},g=()=>{l(),e(jo())},d=i.length==0?"":i.map((c,m)=>t.jsx(Gr,{open:!0,autoHideDuration:6e3,onClose:()=>dn(m),anchorOrigin:{vertical:"bottom",horizontal:"center"},children:t.jsx(V,{onClose:()=>dn(m),severity:c.type,sx:{width:"100%"},children:c.message})},c.id));return t.jsxs(v,{sx:{display:"flex",flexDirection:"column",minHeight:"100vh",bgcolor:"background.default"},children:[n&&t.jsxs(Pe,{maxWidth:"lg",sx:{display:"flex",justifyContent:"flex-end",pt:{xs:1,sm:2}},children:[t.jsx(B,{id:"account-button",onClick:u,color:"inherit",size:"small",endIcon:t.jsx(qo,{alt:n,src:"/assets/avatar.png",sx:{width:28,height:28,fontSize:14}}),"aria-controls":a?"account-menu":void 0,"aria-haspopup":"true","aria-expanded":a?"true":void 0,sx:{color:"text.secondary",minWidth:0,textTransform:"none"},children:t.jsx(y,{variant:"body2",component:"span",noWrap:!0,sx:{display:{xs:"none",sm:"inline"},maxWidth:260},children:n})}),t.jsxs(Zo,{id:"account-menu",anchorEl:o,open:a,onClose:l,MenuListProps:{"aria-labelledby":"account-button"},children:[t.jsx(pn,{disabled:!0,children:t.jsx(y,{variant:"body2",children:n})}),t.jsx(pn,{onClick:g,children:"Logout"})]})]}),t.jsxs(v,{component:"main",sx:{flexGrow:1},children:[t.jsx(sr,{}),d]})]})};Jt.__docgenInfo={description:"Layout component for the application",methods:[],displayName:"Layout"};const Le=({currentFormStep:e,formSubmitError:n,isSubmittingForm:o,onCancel:r,onSubmit:a})=>t.jsxs(Oe,{open:e!==null,onClose:r,maxWidth:"sm",fullWidth:!0,children:[t.jsx(Ve,{sx:{pb:1},children:t.jsxs(v,{sx:{display:"flex",flexDirection:"column",gap:f.spacing.cardActionGap},children:[t.jsx(y,{variant:"h6",component:"span",children:"Complete setup"}),t.jsx(y,{variant:"body2",color:"text.secondary",children:(e==null?void 0:e.stepTitle)??"Provide the details needed to finish this connection."})]})}),t.jsxs(ze,{dividers:!0,children:[n&&t.jsxs(V,{severity:"error",sx:{mb:2},children:[t.jsx(Yt,{children:"Setup could not be saved"}),n]}),e&&t.jsx(cr,{connectionId:e.connectionId,stepTitle:e.stepTitle,stepDescription:e.stepDescription,jsonSchema:e.jsonSchema,uiSchema:e.uiSchema,onSubmit:a,onCancel:r,isSubmitting:o})]})]});Le.__docgenInfo={description:"",methods:[],displayName:"ConnectionSetupDialog",props:{currentFormStep:{required:!0,tsType:{name:"union",raw:"SetupStep | null",elements:[{name:"SetupStep"},{name:"null"}]},description:""},formSubmitError:{required:!0,tsType:{name:"union",raw:"string | null",elements:[{name:"string"},{name:"null"}]},description:""},isSubmittingForm:{required:!0,tsType:{name:"boolean"},description:""},onCancel:{required:!0,tsType:{name:"signature",type:"function",raw:"() => void",signature:{arguments:[],return:{name:"void"}}},description:""},onSubmit:{required:!0,tsType:{name:"signature",type:"function",raw:"(connectionId: string, data: unknown) => void",signature:{arguments:[{type:{name:"string"},name:"connectionId"},{type:{name:"unknown"},name:"data"}],return:{name:"void"}}},description:""}}};function Qt(){const e=K(),n=j(Dt),o=j(Ft),r=j(Ot),a=j(Vt),i=p.useCallback(()=>`${window.location.origin}/connections`,[]),u=p.useCallback(d=>{e(zt({connectorId:d,returnToUrl:i()})).then(c=>{if(c.meta.requestStatus==="fulfilled"){const m=c.payload;ke(m)&&(window.location.href=m.redirect_url)}})},[e,i]),l=p.useCallback((d,c)=>{const m=(o==null?void 0:o.stepId)??"";e(Ut({connectionId:d,stepId:m,data:c,returnToUrl:i()})).then(k=>{if(k.meta.requestStatus==="fulfilled"){const x=k.payload;ke(x)&&(window.location.href=x.redirect_url)}})},[e,o,i]),g=p.useCallback(()=>{e(o?$t(o.connectionId):Bt())},[e,o]);return{connect:u,currentFormStep:o,formSubmitError:a,isConnecting:n,isSubmittingForm:r,submitForm:l,cancelForm:g}}const ns={h1:({children:e})=>t.jsx(y,{variant:"h4",component:"h2",sx:{mt:4,mb:2},children:e}),h2:({children:e})=>t.jsx(y,{variant:"h5",component:"h2",sx:{mt:4,mb:2},children:e}),h3:({children:e})=>t.jsx(y,{variant:"h6",component:"h3",sx:{mt:3,mb:1.5},children:e}),p:({children:e})=>t.jsx(y,{variant:"body1",sx:{mb:2,color:"text.primary"},children:e}),a:({children:e,href:n})=>t.jsx(y,{component:"a",href:n,target:"_blank",rel:"noreferrer",color:"primary",sx:{fontWeight:600},children:e}),table:({children:e})=>t.jsx(v,{component:"table",sx:{width:"100%",borderCollapse:"collapse",my:3,overflow:"hidden",border:1,borderColor:"divider",borderRadius:f.radius.card},children:e}),th:({children:e})=>t.jsx(v,{component:"th",sx:{textAlign:"left",p:1.5,bgcolor:"action.hover",borderBottom:1,borderColor:"divider"},children:t.jsx(y,{component:"span",variant:"subtitle2",children:e})}),td:({children:e})=>t.jsx(v,{component:"td",sx:{p:1.5,borderTop:1,borderColor:"divider",verticalAlign:"top"},children:t.jsx(y,{component:"span",variant:"body2",children:e})}),img:({alt:e,src:n})=>t.jsx(v,{component:"img",alt:e,src:n,sx:{display:"block",width:"100%",maxHeight:320,objectFit:"cover",borderRadius:f.radius.card,border:1,borderColor:"divider",my:3}}),code:({children:e})=>t.jsx(y,{component:"code",sx:{bgcolor:"action.hover",borderRadius:f.radius.control,fontFamily:"monospace",fontSize:f.markdown.codeFontSize,px:.75,py:.25},children:e})},ts=e=>e.startsWith("data:image/")?e:Go(e),os=e=>{const n=e.split(/[^a-zA-Z0-9]+/).filter(Boolean);return n.length===0?"AP":n.slice(0,2).map(o=>o[0].toUpperCase()).join("")},eo=({connectorId:e})=>{const n=K(),o=ar(),r=e??o.connectorId,a=j(Ze),i=j(Ye),u=j(Ke),{cancelForm:l,connect:g,currentFormStep:d,formSubmitError:c,isConnecting:m,isSubmittingForm:k,submitForm:x}=Qt();p.useEffect(()=>{i==="idle"&&n(Xe())},[n,i]);const s=p.useMemo(()=>a.find(b=>b.id===r),[a,r]),h=(s==null?void 0:s.description)||(s==null?void 0:s.highlight)||"";let T;return i==="loading"||i==="idle"?T=t.jsxs(v,{children:[t.jsx(De,{variant:"text",width:"45%",height:56}),t.jsx(De,{variant:"text",width:"70%",height:28}),t.jsx(De,{variant:"rectangular",height:240,sx:{mt:4,borderRadius:f.radius.card}})]}):i==="failed"?T=t.jsx(V,{severity:"error",children:u}):s?T=t.jsxs(t.Fragment,{children:[t.jsxs(v,{sx:{display:"grid",gridTemplateColumns:{xs:"1fr",md:"minmax(0, 1fr) auto"},gap:f.spacing.sectionGap,alignItems:"center",mb:f.spacing.sectionGap},children:[t.jsxs(v,{sx:{display:"flex",gap:2.5,alignItems:"center",minWidth:0},children:[s.logo?t.jsx(v,{component:"img",src:s.logo,alt:`${s.display_name} logo`,sx:{width:88,height:88,objectFit:"contain",bgcolor:"background.default",border:1,borderColor:"divider",borderRadius:f.radius.card,p:1.5,flexShrink:0}}):t.jsx(v,{role:"img","aria-label":`${s.display_name} logo`,sx:{width:88,height:88,borderRadius:f.radius.card,bgcolor:"primary.dark",color:"primary.contrastText",display:"flex",alignItems:"center",justifyContent:"center",flexShrink:0},children:t.jsx(y,{variant:"h4",component:"span",sx:{fontWeight:700},children:os(s.display_name)})}),t.jsxs(v,{sx:{minWidth:0},children:[t.jsx(y,{variant:"h3",component:"h1",sx:{mb:1},children:s.display_name}),s.highlight&&t.jsx(y,{variant:"body1",color:"text.secondary",children:s.highlight})]})]}),t.jsxs(v,{sx:{display:"flex",alignItems:"center",gap:1.5,justifyContent:{xs:"flex-start",md:"flex-end"}},children:[m&&t.jsx(Ee,{size:22}),t.jsx(B,{variant:"contained",onClick:()=>g(s.id),disabled:m,children:"Connect"})]})]}),t.jsx(lr,{sx:{mb:f.spacing.sectionGap}}),t.jsx(Gt,{elevation:0,sx:{p:{xs:0,sm:0},bgcolor:"transparent","& ul, & ol":{pl:3,mb:2},"& li":{marginBottom:.75},"& pre":{bgcolor:"action.hover",p:2,borderRadius:f.radius.card,overflowX:"auto"}},children:t.jsx(Bo,{remarkPlugins:[No],components:ns,urlTransform:ts,children:h})})]}):T=t.jsx(V,{severity:"warning",children:"Connector not found."}),t.jsxs(Pe,{sx:{py:f.spacing.pageY},children:[t.jsx(v,{sx:{mb:f.spacing.sectionGap},children:t.jsx(B,{component:rn,to:"/connectors",startIcon:t.jsx(Xt,{}),children:"Back to connectors"})}),T,t.jsx(Le,{currentFormStep:d,formSubmitError:c,isSubmittingForm:k,onCancel:l,onSubmit:x})]})};eo.__docgenInfo={description:"",methods:[],displayName:"ConnectorDetail",props:{connectorId:{required:!1,tsType:{name:"string"},description:""}}};const no=()=>{const e=K(),n=tn(),o=j(Ze),r=j(Ye),a=j(Ke),{cancelForm:i,connect:u,currentFormStep:l,formSubmitError:g,isConnecting:d,isSubmittingForm:c,submitForm:m}=Qt();p.useEffect(()=>{r==="idle"&&e(Xe())},[r,e]);const k=p.useCallback(s=>{n(`/connectors/${encodeURIComponent(s)}`)},[n]);let x;return r==="loading"?x=t.jsx(L,{container:!0,spacing:f.spacing.gridGap,children:[1,2,3,4].map(s=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Zt,{})},s))}):r==="failed"?x=t.jsx(V,{severity:"error",children:a}):o.length===0?x=t.jsx(v,{sx:{textAlign:"center",py:f.spacing.pageY},children:t.jsx(y,{variant:"h6",color:"text.secondary",children:"No connectors available"})}):x=t.jsx(L,{container:!0,spacing:f.spacing.gridGap,children:o.map(s=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(qt,{connector:s,onConnect:u,onDetails:k,isConnecting:d})},s.id))}),t.jsxs(Pe,{sx:{py:f.spacing.pageY},children:[t.jsxs(v,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:f.spacing.headerGap,mb:f.spacing.sectionGap},children:[t.jsx(y,{variant:"h4",component:"h1",children:"Available Connectors"}),t.jsxs(v,{sx:{display:"flex",alignItems:"center",gap:f.spacing.headerGap},children:[d&&t.jsxs(v,{sx:{display:"flex",alignItems:"center"},children:[t.jsx(Ee,{size:24,sx:{mr:1}}),t.jsx(y,{variant:"body2",color:"text.secondary",children:"Connecting..."})]}),t.jsx(B,{component:rn,to:"/connections",startIcon:t.jsx(Xt,{}),sx:{alignSelf:{xs:"flex-start",sm:"center"}},children:"Back to Connections"})]})]}),x,t.jsx(Le,{currentFormStep:l,formSubmitError:g,isSubmittingForm:c,onCancel:i,onSubmit:m})]})};no.__docgenInfo={description:"Component to display a list of available connectors",methods:[],displayName:"ConnectorList"};const to=()=>{const e=K(),[n,o]=es(),r=j(wo),a=j(ko),i=j(To),u=j(Ze),l=j(Ye),g=j(Ke),d=j(Dt),c=j(Ft),m=j(Ot),k=j(Vt),x=j(Eo),s=j(Ao),h=j(Ro);p.useEffect(()=>{a==="idle"&&e(Me()),l==="idle"&&e(Xe())},[a,l,e]),p.useEffect(()=>{const S=n.get("setup"),E=n.get("connection_id");S==="pending"&&E&&(e(un(E)),n.delete("setup"),n.delete("connection_id"),o(n,{replace:!0}))},[n,o,e]),p.useEffect(()=>{if(!x)return;const S=window.setInterval(()=>{e(un(x))},2e3);return()=>window.clearInterval(S)},[x,e]);const T=p.useCallback((S,E)=>{const M=(c==null?void 0:c.stepId)??"";e(Ut({connectionId:S,stepId:M,data:E,returnToUrl:window.location.href})).then(q=>{if(q.meta.requestStatus==="fulfilled"){const Z=q.payload;ke(Z)?window.location.href=Z.redirect_url:e(Me())}})},[e,c]),b=p.useCallback(()=>{const S=c==null?void 0:c.connectionId,E=S?r.find(M=>M.id===S):void 0;E&&E.state===Te.CONFIGURED&&e(Io(E.id)),e(Bt())},[e,c,r]),w=p.useCallback(()=>{s&&e(Po({connectionId:s.connectionId,returnToUrl:window.location.href})).then(S=>{if(S.meta.requestStatus==="fulfilled"){const E=S.payload;E.type==="redirect"&&E.redirect_url&&(window.location.href=E.redirect_url)}})},[e,s]),C=p.useCallback(()=>{s&&e($t(s.connectionId)).then(()=>{e(Lo()),e(Me())})},[e,s]),z=p.useCallback(S=>{e(zt({connectorId:S,returnToUrl:`${window.location.origin}/connections`})).then(E=>{if(E.meta.requestStatus==="fulfilled"){const M=E.payload;ke(M)&&(window.location.href=M.redirect_url)}})},[e]),U=()=>l==="loading"||l==="idle"?t.jsx(L,{container:!0,spacing:f.spacing.gridGap,children:[1,2,3,4].map(S=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Zt,{})},`connector-skeleton-${S}`))}):l==="failed"?t.jsx(V,{severity:"error",children:g}):u.length===0?t.jsx(v,{sx:{py:3},children:t.jsx(y,{color:"text.secondary",children:"No connectors are available right now."})}):t.jsx(L,{container:!0,spacing:f.spacing.gridGap,children:u.map(S=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(qt,{connector:S,onConnect:z,isConnecting:d})},S.id))});let N;return a==="loading"?N=t.jsx(L,{container:!0,spacing:f.spacing.gridGap,children:[1,2,3,4].map(S=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(Mo,{})},`connection-skeleton-${S}`))}):a==="failed"?N=t.jsx(V,{severity:"error",children:i}):r.length===0?N=t.jsxs(t.Fragment,{children:[t.jsxs(v,{sx:{border:1,borderColor:f.card.borderColor,borderRadius:f.radius.panel,bgcolor:f.card.surface,mb:f.spacing.sectionGap,p:f.spacing.panelPadding},children:[t.jsx(y,{variant:"h5",component:"h2",gutterBottom:!0,children:"Connect your first application"}),t.jsx(y,{color:"text.secondary",sx:{maxWidth:680},children:"Choose a connector below to create a connection. Once connected, it will appear here for ongoing setup, health, and management."}),d&&t.jsxs(v,{sx:{display:"flex",alignItems:"center",mt:3},children:[t.jsx(Ee,{size:24,sx:{mr:1}}),t.jsx(y,{variant:"body2",color:"text.secondary",children:"Starting connection..."})]})]}),t.jsxs(v,{children:[t.jsx(y,{variant:"h6",component:"h2",sx:{mb:2},children:"Available connectors"}),U()]})]}):N=t.jsx(L,{container:!0,spacing:f.spacing.gridGap,children:r.map(S=>t.jsx(L,{size:{xs:12,sm:6,md:4,lg:3},children:t.jsx(_o,{connection:S})},S.id))}),t.jsxs(Pe,{sx:{py:f.spacing.pageY},children:[t.jsxs(v,{sx:{display:"flex",justifyContent:"space-between",alignItems:{xs:"flex-start",sm:"center"},flexDirection:{xs:"column",sm:"row"},gap:f.spacing.headerGap,mb:f.spacing.sectionGap},children:[t.jsx(y,{variant:"h4",component:"h1",children:"Your Connections"}),r.length>0&&t.jsx(B,{variant:"contained",color:"primary",startIcon:t.jsx(dr,{}),component:rn,to:"/connectors",children:"Connect More"})]}),N,t.jsx(Le,{currentFormStep:c,formSubmitError:k,isSubmittingForm:m,onCancel:b,onSubmit:T}),t.jsxs(Oe,{open:x!==null,maxWidth:"xs",fullWidth:!0,children:[t.jsx(Ve,{sx:{pb:1},children:"Verifying connection"}),t.jsx(ze,{dividers:!0,children:t.jsxs(v,{sx:{display:"flex",flexDirection:"column",alignItems:"center",gap:f.spacing.headerGap,py:3},children:[t.jsx(Nr,{color:"primary",sx:{fontSize:40}}),t.jsxs(v,{sx:{textAlign:"center"},children:[t.jsx(y,{variant:"subtitle1",component:"p",children:"Checking credentials"}),t.jsx(y,{variant:"body2",color:"text.secondary",children:"AuthProxy is confirming that this connection can reach the provider."})]}),t.jsx(Lr,{sx:{width:"100%"}})]})})]}),t.jsxs(Oe,{open:s!==null,onClose:C,maxWidth:"sm",fullWidth:!0,children:[t.jsx(Ve,{sx:{pb:1},children:t.jsxs(v,{sx:{display:"flex",alignItems:"center",gap:1},children:[t.jsx(ur,{color:"error"}),t.jsx(y,{variant:"h6",component:"span",children:"Connection verification failed"})]})}),t.jsxs(ze,{dividers:!0,children:[t.jsxs(V,{severity:"error",sx:{mb:2},children:[t.jsx(Yt,{children:"Provider check failed"}),(s==null?void 0:s.message)??"Verification failed"]}),t.jsx(y,{variant:"body2",color:"text.secondary",children:s!=null&&s.canRetry?"Retry setup to run verification again. Cancel setup deletes this unfinished connection.":"Cancel setup to delete this unfinished connection, then start again from the connector."})]}),t.jsxs(Yo,{children:[t.jsx(B,{onClick:C,disabled:h,children:"Cancel setup"}),(s==null?void 0:s.canRetry)&&t.jsx(B,{onClick:w,disabled:h,variant:"contained",startIcon:h?t.jsx(Ee,{size:16}):void 0,children:h?"Retrying setup...":"Retry setup"})]})]})]})};to.__docgenInfo={description:"Component to display a list of connections",methods:[],displayName:"ConnectionList"};const W=(e,n,o="#ffffff")=>{const r=e.split(/\s+/).map(i=>i[0]).join("").slice(0,2).toUpperCase(),a=`<svg xmlns="http://www.w3.org/2000/svg" width="280" height="140" viewBox="0 0 280 140" role="img" aria-label="${e} logo"><rect width="280" height="140" rx="8" fill="${n}"/><text x="50%" y="54%" text-anchor="middle" dominant-baseline="middle" fill="${o}" font-family="Inter, Arial, sans-serif" font-size="42" font-weight="700">${r}</text></svg>`;return`data:image/svg+xml,${encodeURIComponent(a)}`},O=[{id:"google-drive",namespace:"root",version:1,state:P.ACTIVE,display_name:"Google Drive",description:"Have the agent track your work in Google Drive.",highlight:"Have the agent track your work in Google Drive.",logo:W("Google Drive","#188038"),has_configure:!1,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"greenhouse",namespace:"root",version:1,state:P.ACTIVE,display_name:"Greenhouse",description:"This integration pushes candidates to greenhouse.",highlight:"This integration pushes candidates to greenhouse.",logo:W("Greenhouse","#24a47f"),has_configure:!1,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"google-calendar",namespace:"root",version:1,state:P.ACTIVE,display_name:"Google Calendar",description:`Google Calendar lets agents coordinate scheduling work without needing direct access to your primary app.

![Calendar workflow preview](/calendar-workflow-preview.svg)

### What agents can do

| Capability | Supported |
| --- | --- |
| Find open time | Yes |
| Create and update events | Yes |
| Read attendee responses | Yes |
| Manage private event details | No |

Use this connector when the assistant should propose meeting times, create holds, or keep follow-up work attached to calendar events.`,highlight:"Coordinate meetings, availability, and follow-up from Google Calendar.",logo:W("Google Calendar","#1a73e8"),has_configure:!0,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"gmail",namespace:"root",version:1,state:P.ACTIVE,display_name:"GMail",description:"Have the agent respond to your emails without you needing to be involved. Like magic.",highlight:"Have the agent respond to your emails without you needing to be involved. Like magic.",logo:W("GMail","#d93025"),has_configure:!1,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"pipedrive",namespace:"root",version:1,state:P.ACTIVE,display_name:"pipedrive",description:"Allow our agent to handle your sales support.",highlight:"Allow our agent to handle your sales support.",logo:W("pipedrive","#017a5e"),has_configure:!1,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"},{id:"asana",namespace:"root",version:1,state:P.ACTIVE,display_name:"Asana",description:"Allow our agent organize your work.",highlight:"Allow our agent organize your work.",logo:W("Asana","#f06a6a"),has_configure:!1,versions:1,states:[P.ACTIVE],created_at:"2024-01-01T00:00:00Z",updated_at:"2024-01-01T00:00:00Z"}],$=(e,n={})=>({id:`cxn_${e.id}`,namespace:"root",connector:e,state:Te.CONFIGURED,health_state:Je.HEALTHY,created_at:"2024-04-01T12:00:00Z",updated_at:"2024-04-01T12:00:00Z",...n}),rs=[$(O[0]),$(O[2],{health_state:Je.UNHEALTHY}),$(O[5],{state:Te.SETUP}),$(O[4],{state:Te.DISABLED})],sn={connectionId:"cxn_google-calendar",stepId:"select-calendar",stepTitle:"Select a Calendar",stepDescription:"Choose which Google Calendar the agent should manage.",currentStep:0,totalSteps:2,jsonSchema:{type:"object",required:["calendar_id"],properties:{calendar_id:{type:"string",title:"Calendar",enum:["primary","product","support"]}}},uiSchema:{type:"VerticalLayout",elements:[{type:"Control",scope:"#/properties/calendar_id"}]}},A={items:rs,status:"succeeded",error:null,initiatingConnection:!1,initiationError:null,disconnectingConnection:!1,disconnectionError:null,currentTaskId:null,currentFormStep:null,submittingForm:!1,formSubmitError:null,verifyingConnectionId:null,verifyError:null,retryingConnection:!1};function ss({route:e,connectorsState:n={items:O,status:"succeeded",error:null},connectionsState:o=A}){const r=Do({reducer:Fo({auth:$o,connectors:Uo,connections:zo,toasts:Vo}),preloadedState:{auth:{actor_id:"actor_storybook",status:"authenticated"},connectors:n,connections:o,toasts:{items:[]}}});return t.jsx(Oo,{store:r,children:t.jsxs(er,{theme:Ko,children:[t.jsx(Sr,{}),t.jsx(ir,{children:t.jsx(hn,{element:t.jsx(Jt,{}),children:t.jsx(hn,{path:"*",element:e==="/connectors"?t.jsx(no,{}):e==="/connector-detail"?t.jsx(eo,{connectorId:"google-calendar"}):t.jsx(to,{})})})})]})})}const As={title:"Pages/Marketplace",component:ss,parameters:{layout:"fullscreen"}},_e={viewport:{defaultViewport:"marketplaceMobile"}},J={viewport:{defaultViewport:"marketplaceTablet"}},ne={args:{route:"/connectors"}},te={args:{route:"/connector-detail"}},oe={args:{route:"/connector-detail"},parameters:_e},re={args:{route:"/connectors",connectorsState:{items:[],status:"loading",error:null}}},se={args:{route:"/connections"}},ae={args:{route:"/connections"},parameters:_e},ie={args:{route:"/connections"},parameters:J},ce={args:{route:"/connections",connectionsState:{...A,items:[$(O[2],{health_state:Je.UNHEALTHY})]}}},le={args:{route:"/connections",connectionsState:{...A,items:[$(O[2]),$(O[0])]}}},de={args:{route:"/connections",connectionsState:{...A,items:[]}}},ue={args:{route:"/connections",connectionsState:{...A,items:[]}},parameters:_e},pe={args:{route:"/connections",connectionsState:{...A,items:[]}},parameters:J},me={args:{route:"/connections",connectorsState:{items:[],status:"loading",error:null},connectionsState:{...A,items:[]}}},ge={args:{route:"/connectors"},parameters:_e},fe={args:{route:"/connectors"},parameters:J},he={args:{route:"/connections",connectionsState:{...A,currentFormStep:sn}}},be={args:{route:"/connections",connectionsState:{...A,currentFormStep:sn}},parameters:J},xe={args:{route:"/connections",connectionsState:{...A,currentFormStep:sn,submittingForm:!0}}},ye={args:{route:"/connections",connectionsState:{...A,verifyingConnectionId:"cxn_google-calendar"}}},Ce={args:{route:"/connections",connectionsState:{...A,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}}},ve={args:{route:"/connections",connectionsState:{...A,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0}}},parameters:J},Se={args:{route:"/connections",connectionsState:{...A,verifyError:{connectionId:"cxn_google-calendar",message:"Calendar API rejected the saved credentials.",canRetry:!0},retryingConnection:!0}}},je={args:{route:"/connections",connectionsState:{...A,verifyError:{connectionId:"cxn_google-calendar",message:"The provider rejected this setup and it cannot be retried.",canRetry:!1}}}};var Cn,vn,Sn;ne.parameters={...ne.parameters,docs:{...(Cn=ne.parameters)==null?void 0:Cn.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  }
}`,...(Sn=(vn=ne.parameters)==null?void 0:vn.docs)==null?void 0:Sn.source}}};var jn,wn,kn;te.parameters={...te.parameters,docs:{...(jn=te.parameters)==null?void 0:jn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  }
}`,...(kn=(wn=te.parameters)==null?void 0:wn.docs)==null?void 0:kn.source}}};var Tn,En,An;oe.parameters={...oe.parameters,docs:{...(Tn=oe.parameters)==null?void 0:Tn.docs,source:{originalSource:`{
  args: {
    route: '/connector-detail'
  },
  parameters: mobileViewport
}`,...(An=(En=oe.parameters)==null?void 0:En.docs)==null?void 0:An.source}}};var Rn,In,Pn;re.parameters={...re.parameters,docs:{...(Rn=re.parameters)==null?void 0:Rn.docs,source:{originalSource:`{
  args: {
    route: '/connectors',
    connectorsState: {
      items: [],
      status: 'loading',
      error: null
    }
  }
}`,...(Pn=(In=re.parameters)==null?void 0:In.docs)==null?void 0:Pn.source}}};var Ln,_n,Mn;se.parameters={...se.parameters,docs:{...(Ln=se.parameters)==null?void 0:Ln.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  }
}`,...(Mn=(_n=se.parameters)==null?void 0:_n.docs)==null?void 0:Mn.source}}};var Dn,Fn,On;ae.parameters={...ae.parameters,docs:{...(Dn=ae.parameters)==null?void 0:Dn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: mobileViewport
}`,...(On=(Fn=ae.parameters)==null?void 0:Fn.docs)==null?void 0:On.source}}};var Vn,zn,Un;ie.parameters={...ie.parameters,docs:{...(Vn=ie.parameters)==null?void 0:Vn.docs,source:{originalSource:`{
  args: {
    route: '/connections'
  },
  parameters: tabletViewport
}`,...(Un=(zn=ie.parameters)==null?void 0:zn.docs)==null?void 0:Un.source}}};var $n,Bn,Gn;ce.parameters={...ce.parameters,docs:{...($n=ce.parameters)==null?void 0:$n.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2], {
        health_state: ConnectionHealthState.UNHEALTHY
      })]
    }
  }
}`,...(Gn=(Bn=ce.parameters)==null?void 0:Bn.docs)==null?void 0:Gn.source}}};var Nn,Hn,Wn;le.parameters={...le.parameters,docs:{...(Nn=le.parameters)==null?void 0:Nn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: [connectionFor(connectors[2]), connectionFor(connectors[0])]
    }
  }
}`,...(Wn=(Hn=le.parameters)==null?void 0:Hn.docs)==null?void 0:Wn.source}}};var qn,Zn,Yn;de.parameters={...de.parameters,docs:{...(qn=de.parameters)==null?void 0:qn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  }
}`,...(Yn=(Zn=de.parameters)==null?void 0:Zn.docs)==null?void 0:Yn.source}}};var Kn,Xn,Jn;ue.parameters={...ue.parameters,docs:{...(Kn=ue.parameters)==null?void 0:Kn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: mobileViewport
}`,...(Jn=(Xn=ue.parameters)==null?void 0:Xn.docs)==null?void 0:Jn.source}}};var Qn,et,nt;pe.parameters={...pe.parameters,docs:{...(Qn=pe.parameters)==null?void 0:Qn.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      items: []
    }
  },
  parameters: tabletViewport
}`,...(nt=(et=pe.parameters)==null?void 0:et.docs)==null?void 0:nt.source}}};var tt,ot,rt;me.parameters={...me.parameters,docs:{...(tt=me.parameters)==null?void 0:tt.docs,source:{originalSource:`{
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
}`,...(rt=(ot=me.parameters)==null?void 0:ot.docs)==null?void 0:rt.source}}};var st,at,it;ge.parameters={...ge.parameters,docs:{...(st=ge.parameters)==null?void 0:st.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: mobileViewport
}`,...(it=(at=ge.parameters)==null?void 0:at.docs)==null?void 0:it.source}}};var ct,lt,dt;fe.parameters={...fe.parameters,docs:{...(ct=fe.parameters)==null?void 0:ct.docs,source:{originalSource:`{
  args: {
    route: '/connectors'
  },
  parameters: tabletViewport
}`,...(dt=(lt=fe.parameters)==null?void 0:lt.docs)==null?void 0:dt.source}}};var ut,pt,mt;he.parameters={...he.parameters,docs:{...(ut=he.parameters)==null?void 0:ut.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  }
}`,...(mt=(pt=he.parameters)==null?void 0:pt.docs)==null?void 0:mt.source}}};var gt,ft,ht;be.parameters={...be.parameters,docs:{...(gt=be.parameters)==null?void 0:gt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep
    }
  },
  parameters: tabletViewport
}`,...(ht=(ft=be.parameters)==null?void 0:ft.docs)==null?void 0:ht.source}}};var bt,xt,yt;xe.parameters={...xe.parameters,docs:{...(bt=xe.parameters)==null?void 0:bt.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      currentFormStep: setupStep,
      submittingForm: true
    }
  }
}`,...(yt=(xt=xe.parameters)==null?void 0:xt.docs)==null?void 0:yt.source}}};var Ct,vt,St;ye.parameters={...ye.parameters,docs:{...(Ct=ye.parameters)==null?void 0:Ct.docs,source:{originalSource:`{
  args: {
    route: '/connections',
    connectionsState: {
      ...baseConnectionsState,
      verifyingConnectionId: 'cxn_google-calendar'
    }
  }
}`,...(St=(vt=ye.parameters)==null?void 0:vt.docs)==null?void 0:St.source}}};var jt,wt,kt;Ce.parameters={...Ce.parameters,docs:{...(jt=Ce.parameters)==null?void 0:jt.docs,source:{originalSource:`{
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
}`,...(kt=(wt=Ce.parameters)==null?void 0:wt.docs)==null?void 0:kt.source}}};var Tt,Et,At;ve.parameters={...ve.parameters,docs:{...(Tt=ve.parameters)==null?void 0:Tt.docs,source:{originalSource:`{
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
}`,...(At=(Et=ve.parameters)==null?void 0:Et.docs)==null?void 0:At.source}}};var Rt,It,Pt;Se.parameters={...Se.parameters,docs:{...(Rt=Se.parameters)==null?void 0:Rt.docs,source:{originalSource:`{
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
}`,...(Pt=(It=Se.parameters)==null?void 0:It.docs)==null?void 0:Pt.source}}};var Lt,_t,Mt;je.parameters={...je.parameters,docs:{...(Lt=je.parameters)==null?void 0:Lt.docs,source:{originalSource:`{
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
}`,...(Mt=(_t=je.parameters)==null?void 0:_t.docs)==null?void 0:Mt.source}}};const Rs=["AvailableConnectors","ConnectorOverview","ConnectorOverviewMobile","AvailableConnectorsLoading","ConnectionsPopulated","ConnectionsPopulatedMobile","ConnectionsPopulatedTablet","ConnectionsNeedsAttention","ConnectionsHealthyActions","ConnectionsEmpty","ConnectionsEmptyMobile","ConnectionsEmptyTablet","ConnectionsEmptyLoadingConnectors","AvailableConnectorsMobile","AvailableConnectorsTablet","ConnectionSetupDialog","ConnectionSetupDialogTablet","ConnectionSetupSubmitting","VerifyingConnectionDialog","VerificationFailedDialog","VerificationFailedDialogTablet","VerificationRetryingDialog","VerificationFailedNoRetryDialog"];export{ne as AvailableConnectors,re as AvailableConnectorsLoading,ge as AvailableConnectorsMobile,fe as AvailableConnectorsTablet,he as ConnectionSetupDialog,be as ConnectionSetupDialogTablet,xe as ConnectionSetupSubmitting,de as ConnectionsEmpty,me as ConnectionsEmptyLoadingConnectors,ue as ConnectionsEmptyMobile,pe as ConnectionsEmptyTablet,le as ConnectionsHealthyActions,ce as ConnectionsNeedsAttention,se as ConnectionsPopulated,ae as ConnectionsPopulatedMobile,ie as ConnectionsPopulatedTablet,te as ConnectorOverview,oe as ConnectorOverviewMobile,Ce as VerificationFailedDialog,ve as VerificationFailedDialogTablet,je as VerificationFailedNoRetryDialog,Se as VerificationRetryingDialog,ye as VerifyingConnectionDialog,Rs as __namedExportsOrder,As as default};

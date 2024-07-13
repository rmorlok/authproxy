import Typography from "@mui/material/Typography";
import * as React from "react";
import {Button, Card, CardContent, CardHeader, TextField, InputAdornment, Icon, Grid, Tooltip} from "@mui/material";
import {FiberNew, Close, FastForward} from "@mui/icons-material";
import FingerprintIcon from '@mui/icons-material/Fingerprint';
import ExitToAppIcon from '@mui/icons-material/ExitToApp';
import {Player, Team} from "./api/models";
import {BLUE_TEXT_CSS, RED_TEXT_CSS} from "./colors";
import Avatar from "@mui/material/Avatar";

type Props = {
    tableId: string;
    player: Player;
    players: Player[];
    playTeam: (t: Team) => void;
    makeSpymaster: () => void;
    leaveTable: () => void;
    logout: () => void;
    startGame: () => void;
};

type Column = {
    team: Team;
    display: string;
    css: object,
};

const columns: Column[] = [
    {
        team: 'red',
        display: 'Red',
        css: RED_TEXT_CSS,
    },
    {
        team: 'blue',
        display: 'Blue',
        css: BLUE_TEXT_CSS,
    }
];

export function TeamPicker({
    tableId,
    player,
    players,
    playTeam,
    makeSpymaster,
    leaveTable,
    logout,
    startGame,
}: Props) {
    const hasSpymaster = (color: Team) => {
        return players.findIndex((p) => p.team === color && p.isSpyMaster) !== -1;
    };

    const isMe = (p: Player) => p.user.id === player.user.id;

    const isMyTeam = (color: Team) => {
        return players.findIndex((p) => p.team === color && isMe(p)) !== -1;
    };

    const playersForTeam = (color: Team) => {
        return players
            .filter((p) => p.team === color)
            .sort((a, b) => {
                if(a.isSpyMaster) {
                    return -1;
                } else if (b.isSpyMaster) {
                    return 1;
                } else {
                    return a.user.name.localeCompare(b.user.name);
                }
            });
    };

    return (
        <Card sx={{ width: 600, maxWidth: 600 }} variant={'outlined'}>
            <CardContent>
                <Typography variant="h3" component="div" sx={{display: 'flex', justifyContent: 'center'}}>
                    Pick Teams
                </Typography>
                <Typography variant="h6" component="div" sx={{display: 'flex', justifyContent: 'center'}}>
                    Table ID#{tableId}
                </Typography>
            </CardContent>
            <CardContent>
                <Grid container spacing={2}>
                    {columns.map((c) => (
                        <Grid container xs={6} item={true} key={c.team + 'heading'} direction={'column'}>
                            <Grid container direction={'row'} style={{marginBottom: '10px', justifyContent: 'center'}}>
                                <Typography variant="h5" component="div" sx={{...c.css, display: 'flex'}}>
                                    {c.display} Team
                                </Typography>
                            </Grid>
                            {playersForTeam(c.team).map((p) => (
                                <Grid
                                    container
                                    direction={'row'}
                                    alignItems={'center'}
                                    style={{marginBottom: '5px'}}
                                    item={true}
                                    key={p.user.id}
                                >
                                    <Avatar alt={p.user.name} src={p.user.picture} sx={{marginLeft: '60px'}}/>
                                    &nbsp;&nbsp;
                                    <Typography variant="body1" component="span">
                                        {p.user.name} {isMe(p) ? '(me)' : '' }
                                    </Typography>
                                    &nbsp;
                                    {p.isSpyMaster ? (<Tooltip title="Spymaster"><FingerprintIcon  /></Tooltip>) : (<Icon />)}
                                </Grid>
                            ))}
                            {isMyTeam(c.team) ? '' : (
                                <Grid
                                    container
                                    direction={'row'}
                                    alignItems={'center'}
                                    item={true}
                                    key={c.team + 'play_button'}
                                >
                                    <Button
                                        variant="contained"
                                        endIcon={<FiberNew />}
                                        onClick={() => playTeam(c.team)}
                                        sx={{flex: 1}}
                                    >
                                        Play {c.display}
                                    </Button>
                                </Grid>
                            )}
                            {!(isMyTeam(c.team) && !hasSpymaster(c.team)) ? '' : (
                                <Grid
                                    container
                                    direction={'row'}
                                    alignItems={'center'}
                                    item={true}
                                    key={c.team + 'spymaster_button'}
                                >
                                    <Button
                                        variant="contained"
                                        endIcon={<FingerprintIcon  />}
                                        onClick={makeSpymaster}
                                        sx={{flex: 1}}>
                                        I'm the Spymaster
                                    </Button>
                                </Grid>
                            )}
                        </Grid>
                    ))}
                </Grid>
            </CardContent>
            <CardContent>
                <Typography variant="h5" component="div" sx={{...RED_TEXT_CSS, display: 'flex', justifyContent: 'center'}}>
                    {hasSpymaster('red') ? '' : 'Red needs a spymaster'}
                </Typography>
                <Typography variant="h5" component="div" sx={{...BLUE_TEXT_CSS, display: 'flex', justifyContent: 'center'}}>
                    {hasSpymaster('blue') ? '' : 'Blue needs a spymaster'}
                </Typography>
            </CardContent>
            <CardContent sx={{display: "flex"}}>
                <Button color={'secondary'} startIcon={<Close />} onClick={logout} sx={{flex: 1}}>
                    Logout
                </Button>
                &nbsp;&nbsp;
                <Button variant="contained" endIcon={<ExitToAppIcon />} onClick={leaveTable} sx={{flex: 1}}>
                    Leave Table
                </Button>
                &nbsp;&nbsp;
                <Button variant="contained" endIcon={<FiberNew />} onClick={startGame} sx={{flex: 1}}>
                    Start Game
                </Button>
            </CardContent>
        </Card>
    );
}
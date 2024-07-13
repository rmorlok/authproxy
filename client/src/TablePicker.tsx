import Typography from "@mui/material/Typography";
import * as React from "react";
import {Button, Card, CardContent, CardHeader, TextField, InputAdornment, Icon} from "@mui/material";
import {FiberNew, Close, FastForward} from "@mui/icons-material";
import CheckIcon from '@mui/icons-material/Check';
import {useEffect, useState} from "react";
import {debounceTime, distinctUntilChanged, mergeMap, Subject, tap} from "rxjs";
import CircularProgress from "@mui/material/CircularProgress";

type Props = {
    logout: () => void;
    createTable: () => void;
    joinTable: (id: string) => void;
    validateTableId: (id: string) => Promise<boolean>;
}

export function TablePicker(props: Props) {
    const [tableId, setTableId] = useState('');
    const [loading, setLoading] = useState(false);
    const [validated, setValidated] = useState<{value: string, valid: boolean}>({value: '', valid: false});

    const [tableInputChanged$] = useState(()=>new Subject<string>());

    useEffect(() => {
        const subscription = tableInputChanged$.pipe(
            debounceTime(300),
            distinctUntilChanged(),
            mergeMap(async (v) => {
                setLoading(true);
                const result = {value: v, valid: await props.validateTableId(v)};
                setLoading(false);
                return result;
            })
        ).subscribe(setValidated);
    }, []);

    const tableFiledChanged = (e: React.ChangeEvent<HTMLInputElement>) => {
        setTableId(e.target.value);
        tableInputChanged$.next(e.target.value)
    };

    return (
        <Card sx={{ width: 450, maxWidth: 450 }} variant={'outlined'}>
            <CardContent>
                <Typography variant="h5" component="div" sx={{display: 'flex', justifyContent: 'center'}}>
                    Join or Create a Table
                </Typography>
            </CardContent>
            <CardContent sx={{display: "flex"}}>
                <TextField
                    label="Enter Table ID"
                    variant="filled"
                    value={tableId}
                    onChange={tableFiledChanged}
                    sx={{flex: 1}}
                    InputProps={{endAdornment: (
                        <InputAdornment position="end">
                            {!loading && validated.value === tableId && validated.valid ? <CheckIcon
                                aria-label="valid"
                                style={{ color: "#0a0" }}
                            /> : ''}
                            {loading  ? <CircularProgress size="24px" /> : ''}
                            {!loading && !(validated.value === tableId && validated.valid) ? <Icon /> : ''}
                        </InputAdornment>
                        )}}
                />
                <Button
                    color={'success'}
                    variant="contained"
                    endIcon={<FastForward />}
                    disabled={tableId !== validated.value || !validated.valid}
                    onClick={() => { props.joinTable(tableId); }}
                    sx={{marginLeft: "10px"}}
                >
                    Join Table
                </Button>
            </CardContent>
            <CardContent sx={{display: "flex"}}>
                <Button color={'secondary'} startIcon={<Close />} onClick={props.logout} sx={{flex: 1}}>
                    Logout
                </Button>
                <Button variant="contained" endIcon={<FiberNew />} onClick={props.createTable} sx={{flex: 1}}>
                    Create Table
                </Button>
            </CardContent>
        </Card>
    );
}
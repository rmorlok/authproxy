import { useSelector, useDispatch } from 'react-redux'
import * as React from "react";
import {
    decrement,
    increment,
    incrementByAmount,
    incrementAsync,
    selectCount, AppDispatch,
} from './store'
import Box from "@mui/material/Box";
import {Button} from "@mui/material";

// This is a dummy component for testing

export function Counter(props: any) {
    const count = useSelector(selectCount);
    const dispatch = useDispatch<AppDispatch>();

    return (
        <Box>
            <Button variant="outlined" onClick={() => dispatch(incrementAsync(2))}>Current Count {count}</Button>
        </Box>
    );
}
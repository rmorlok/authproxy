import * as React from "react";
import {Team, WordCardModel} from "./api/models";
import Card from '@mui/material/Card';
import CardContent from '@mui/material/CardContent';
import Typography from "@mui/material/Typography";

type Props = {
    card: WordCardModel;
    click: (word: string) => void;
    isSpymaster: boolean;
    myTeam: Team;
};

export function WordCard({card, click, isSpymaster, myTeam}: Props) {
    let color = null;
    if (isSpymaster) {
        
    } else {

    }

    return (
        <Card onClick={() => click(card.word)} style={{aspectRatio: '1 / 1'}}>
            <CardContent style={{alignContent: 'center', justifyContent: 'center'}}>
                <Typography variant="h5" component="div">
                    {card.word}
                </Typography>
            </CardContent>
        </Card>
    );
}
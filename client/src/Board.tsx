import * as React from "react";
import {Team, BoardModel} from "./api/models";
import {Card} from "@mui/material";
import {WordCard} from "./WordCard";

type Props = {
    board: BoardModel;
    revealCard: (card: string) => void;
    isSpymaster: boolean;
    myTeam: Team;
};

export function Board({board, revealCard, isSpymaster, myTeam}: Props) {
    return (
        {
            board.map((row) => {
                return row.map((card) => (
                    <WordCard
                        card={card}
                        click={revealCard}
                        isSpymaster={isSpymaster}
                        myTeam={myTeam}/>
                ))
            })
        }
    );
}
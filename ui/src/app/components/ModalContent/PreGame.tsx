import React, {FC} from 'react';
import {GameVariantText} from "@components/GameVariantCard/GameVariantText";
import {FontSizes, GameModes} from "@utils/constants";
import {bgColors, textColors} from "@utils/styles/colors";
import {Button} from "@components/Button/Button";
import {GameInvite} from "@app/components/GameInvite/GameInvite";
import {RatedGame} from "@app/querys/FetchRatedPools";

const optionPadding = "pt-4";

interface PreGameProps {
    gameMode: GameModes;
    gameType: RatedGame;
}

export const PreGame: FC<PreGameProps> = (props) => {
    const lastSpaceIdx = props.gameType.name.lastIndexOf(" ");

    return (
        <div
            className="mt-8 text-center sm:mt-0 sm:text-left w-full flex flex-col justify-center items-center"
        >
            <h3 className="text-2xl leading-6 font-medium text-gray-900 text-center" id="modal-headline">
                {props.gameMode === GameModes.PlayOnline ?
                    "Finding Game" :
                    "Waiting for Opponent"}
            </h3>

            {/* Loading spinner */}
            <div className="lds-dual-ring mt-6"/>

            <GameVariantText
                className={`${optionPadding}`}
                nameOnSameLine
                timeControlFontSize={FontSizes.TwoXL}
                timeControl={props.gameType.name.substring(0, lastSpaceIdx)}
                variantName={props.gameType.name.substring(lastSpaceIdx)}
                textColor={textColors.green["500"]}
            />

            {props.gameMode === GameModes.PlayAFriend ?
                <GameInvite inviteLink="ABC123"/> : null}

            {/* Action button */}
            <Button
                className="mt-4 px-8 py-2"
                roundSides
                bgColor={bgColors.yellow["300"]}
            >
                Cancel
            </Button>
        </div>
    );
};
import React, {FC} from 'react';
import {Input} from "@components/Input/Input";
import {Button} from "@components/Button/Button";
import CopyIcon from "@assets/images/copy-regular.svg";
import InfoIcon from "@assets/images/info-circle-solid.svg";
import {bgColors} from "@utils/styles/colors";
import {Tooltip} from "@components/Tooltip/Tooltip";

interface GameInviteProps {
    inviteLink: string
}

export const GameInvite: FC<GameInviteProps> = (props) => {
    return (
        <Input
            centerLabel
            inputClassName="px-4 py-2"
            value={props.inviteLink}
            label={(
                <span className="flex justify-center items-center">
                    Invite Link
                    <Tooltip
                        tooltipContent="The first person who joins this link will be your opponent."
                    >
                        <InfoIcon className="ml-2" />
                    </Tooltip>
                </span>)}
            rightExtra={
                <Button
                    className="ml-2 hover:bg-green-600"
                    roundSides
                    bgColor={bgColors.green["500"]}
                >
                    <CopyIcon
                        className="text-white"
                        onClick={() => {
                            navigator.clipboard.writeText(props.inviteLink)
                                .then(() => console.log("Successfully copied text!")) // todo popup?
                                .catch(console.error)
                        }}
                    />
                </Button>
            }
        />
    );
};
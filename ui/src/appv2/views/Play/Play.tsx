import React from "react";
import {ContentContainer} from "@appv2/containers/ContentContainer";

export const PlayView = (): JSX.Element => {
    return (
        <ContentContainer
            style={{
                padding: 32
            }}
        >
            <div className="h-full grid grid-rows-2 gap-8">
                <div>Quick Match</div>
                <div className="grid grid-cols-2 gap-8">
                    <div>Join a Game</div>
                    <div className="grid grid-Rows-2 gap-8">
                        <div>Top Small Grid</div>
                        <div>Bottom Small Grid</div>
                    </div>
                </div>
            </div>
        </ContentContainer>
    )
}
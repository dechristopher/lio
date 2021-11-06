import React from "react";
import {ContentContainer} from "@appv2/containers/ContentContainer";

export const Sidebar = (): JSX.Element => {

    return (
        <ContentContainer
            style={{
                padding: 32,
                width: 400,
                // TODO remove - for visualization
                borderRight: "1px solid black"
            }}
        >
            Sidebar
        </ContentContainer>
    )
}
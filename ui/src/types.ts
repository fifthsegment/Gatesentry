interface UserType {
    username : string;
    password : string;
    allowaccess : boolean;
    dataconsumed? : number;
}


type Rule = {
    domain: string;
    timeRestriction: { from: string; to: string;};
    user: string;
    contentSize: number;
    contentType: string;
    action: "allow" | "block";
};


export type { UserType, Rule };